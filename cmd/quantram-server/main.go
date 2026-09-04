// Command quantram-server composes the ingestion, model, Snapshot, gRPC, and
// persistence lifecycles. It owns startup ordering and coordinated shutdown;
// scientific processing and MongoDB schema semantics remain owned by their
// respective internal packages.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/adaptive"
	"quantram/internal/config"
	"quantram/internal/ingestion"
	"quantram/internal/marketfeed"
	"quantram/internal/modelhost"
	"quantram/internal/persistence"
	"quantram/internal/semantics"
	"quantram/internal/server"
	"quantram/internal/snapshot"

	"google.golang.org/grpc"
)

const (
	componentShutdownTimeout = 5 * time.Second
	persistenceCloseTimeout  = 15 * time.Second
)

type grpcLifecycle interface {
	GracefulStop()
	Stop()
}

type snapshotEvaluator interface {
	FinalEvaluate(context.Context) error
}

type persistenceCloser interface {
	Drain(context.Context) error
	Close(context.Context) error
}

func main() {
	if err := run(); err != nil {
		log.Printf("QuanTRAM server stopped with error: %v", err)
		os.Exit(1)
	}
}

// run owns one server invocation from configuration through coordinated close.
// Once persistence creates an Aperture, every return path passes through
// shutdownRuntime before main may exit.
func run() (returnErr error) {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	listener, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", cfg.GRPCPort, err)
	}
	defer listener.Close()

	store, snapshotService, err := newPersistence(cfg)
	if err != nil {
		return fmt.Errorf("create persistence: %w", err)
	}
	persistenceClosed := false
	defer func() {
		if store == nil || persistenceClosed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), persistenceCloseTimeout)
		defer cancel()
		returnErr = errors.Join(returnErr, store.Close(ctx))
	}()

	pipeline, err := newPipeline(cfg, store)
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	host, hostErr := modelhost.New(pipeline, cfg.Symbols, modelhost.Options{
		Mode:     cfg.Model,
		Pricing:  cfg.Pricing,
		Deadline: cfg.ModelDeadline,
		Capture:  store,
	})
	modelUnavailable := false
	if hostErr != nil {
		if !errors.Is(hostErr, modelhost.ErrUnavailable) {
			return fmt.Errorf("model host: %w", hostErr)
		}
		log.Printf("model component unavailable: %v", hostErr)
		modelUnavailable = true
		host = nil
	}

	grpcServer := grpc.NewServer()
	var quantramServer *server.Server
	switch {
	case modelUnavailable:
		quantramServer = server.New(pipeline, modelhost.Unavailable{})
	case host != nil:
		quantramServer = server.New(pipeline, host)
	default:
		quantramServer = server.New(pipeline, nil)
	}
	if dict, err := semantics.LoadEmbedded(); err != nil {
		log.Printf("semantic dictionary unavailable: %v", err)
	} else {
		quantramServer.SetSemantics(dict)
		log.Printf("semantic contract %s (%d terms)", dict.Version(), dict.TermCount())
	}
	quantramServer.SetSnapshotService(snapshotService)
	quantramv1.RegisterMarketFeedServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterIngestionServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterOperationsServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterModelServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterSemanticServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterSnapshotServiceServer(grpcServer, quantramServer)

	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	producerCtx, cancelProducers := context.WithCancel(context.Background())
	snapshotCtx, cancelSnapshot := context.WithCancel(context.Background())
	defer cancelProducers()
	defer cancelSnapshot()

	var producerWG sync.WaitGroup
	var snapshotWG sync.WaitGroup
	if snapshotService != nil {
		snapshotWG.Add(1)
		go func() {
			defer snapshotWG.Done()
			snapshotService.Run(snapshotCtx)
		}()
	}

	producerWG.Add(1)
	go func() {
		defer producerWG.Done()
		if err := pipeline.Run(producerCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("ingestion pipeline stopped: %v", err)
		}
	}()
	if host != nil {
		producerWG.Add(1)
		go func() {
			defer producerWG.Done()
			if err := host.Run(producerCtx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("model host stopped: %v", err)
			}
		}()
	}

	serveResult := make(chan error, 1)
	go func() {
		log.Printf("starting QuanTRAM ingestion gRPC server (port=%s source=%s feed=%s symbols=%v model=%s pricing=%s)", cfg.GRPCPort, cfg.Source, cfg.Feed, cfg.Symbols, cfg.Model, cfg.Pricing)
		serveResult <- grpcServer.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			returnErr = fmt.Errorf("serve gRPC: %w", err)
		}
	case <-shutdownCtx.Done():
		log.Print("shutdown signal received")
	}

	shutdownErr := shutdownRuntime(grpcServer, cancelProducers, &producerWG, snapshotService, cancelSnapshot, &snapshotWG, store)
	persistenceClosed = store != nil
	return errors.Join(returnErr, shutdownErr)
}

// shutdownRuntime establishes the durable close boundary. It stops new gRPC
// work, joins all capture producers, performs one final eligible Snapshot scan,
// joins Snapshot processing, and only then drains and closes persistence.
func shutdownRuntime(
	grpcServer grpcLifecycle,
	cancelProducers context.CancelFunc,
	producerWG *sync.WaitGroup,
	snapshotService snapshotEvaluator,
	cancelSnapshot context.CancelFunc,
	snapshotWG *sync.WaitGroup,
	store persistenceCloser,
) error {
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()

	cancelProducers()
	producerWG.Wait()

	select {
	case <-grpcStopped:
		log.Print("graceful shutdown completed")
	case <-time.After(componentShutdownTimeout):
		log.Print("graceful shutdown timed out; forcing stop")
		grpcServer.Stop()
		<-grpcStopped
	}

	var closeErr error
	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), persistenceCloseTimeout)
		closeErr = errors.Join(closeErr, store.Drain(ctx))
		cancel()
	}
	if closeErr == nil && snapshotService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), componentShutdownTimeout)
		closeErr = snapshotService.FinalEvaluate(ctx)
		cancel()
	}
	cancelSnapshot()
	snapshotWG.Wait()

	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), persistenceCloseTimeout)
		closeErr = errors.Join(closeErr, store.Close(ctx))
		cancel()
	}
	return closeErr
}

func newPipeline(cfg config.Config, capture ingestion.BarCapture) (*ingestion.Pipeline, error) {
	switch cfg.Source {
	case "csv":
		live := marketfeed.NewCSVSource(cfg.CSVPath, cfg.Symbols[0])
		return ingestion.NewPipeline(live, nil, marketfeed.SourceID("csv"), cfg.Symbols, capture), nil
	default:
		creds := marketfeed.Credentials{Key: cfg.APIKey, Secret: cfg.APISecret}
		live := marketfeed.NewAlpacaStream(cfg.StreamURL, cfg.Feed, creds)
		historical := marketfeed.NewAlpacaREST(cfg.DataREST, cfg.Feed, creds)
		return ingestion.NewPipeline(live, historical, marketfeed.SourceID(cfg.Feed), cfg.Symbols, capture), nil
	}
}

func newPersistence(cfg config.Config) (*persistence.AsyncStore, *snapshot.Service, error) {
	if cfg.MongoURI == "" {
		return nil, nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	writer, err := persistence.OpenMongo(ctx, persistence.MongoConfig{
		URI:                     cfg.MongoURI,
		Database:                cfg.MongoDatabase,
		SemanticContractVersion: semantics.ContractVersion,
		ModelVersion:            adaptive.ModelVersionLabel,
		SchemaVersion:           adaptive.SchemaVersion,
	})
	if err != nil {
		return nil, nil, err
	}
	snapshotService := snapshot.NewService(writer, writer, writer.ApertureID(), time.Second)
	return persistence.NewAsyncStore(writer, cfg.MongoQueue), snapshotService, nil
}
