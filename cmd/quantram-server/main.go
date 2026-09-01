package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/config"
	"quantram/internal/ingestion"
	"quantram/internal/marketfeed"
	"quantram/internal/modelhost"
	"quantram/internal/server"

	"google.golang.org/grpc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pipeline, err := newPipeline(cfg)
	if err != nil {
		log.Fatalf("create pipeline: %v", err)
	}

	host, hostErr := modelhost.New(pipeline, cfg.Symbols, modelhost.Options{
		Mode:     cfg.Model,
		Deadline: cfg.ModelDeadline,
	})
	modelUnavailable := false
	if hostErr != nil {
		if !errors.Is(hostErr, modelhost.ErrUnavailable) {
			log.Fatalf("model host: %v", hostErr)
		}
		log.Printf("model component unavailable: %v", hostErr)
		modelUnavailable = true
		host = nil
	}

	listener, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("listen on port %s: %v", cfg.GRPCPort, err)
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
	quantramv1.RegisterMarketFeedServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterIngestionServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterOperationsServiceServer(grpcServer, quantramServer)
	quantramv1.RegisterModelServiceServer(grpcServer, quantramServer)

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	go func() {
		if err := pipeline.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("ingestion pipeline stopped: %v", err)
		}
	}()
	if host != nil {
		go func() {
			if err := host.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("model host stopped: %v", err)
			}
		}()
	}

	serveResult := make(chan error, 1)
	go func() {
		log.Printf("starting QuanTRAM ingestion gRPC server (port=%s source=%s feed=%s symbols=%v model=%s)", cfg.GRPCPort, cfg.Source, cfg.Feed, cfg.Symbols, cfg.Model)
		serveResult <- grpcServer.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Fatalf("serve gRPC: %v", err)
		}
	case <-ctx.Done():
		log.Print("shutdown signal received")
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
			log.Print("graceful shutdown completed")
		case <-time.After(5 * time.Second):
			log.Print("graceful shutdown timed out; forcing stop")
			grpcServer.Stop()
		}
	}
}

func newPipeline(cfg config.Config) (*ingestion.Pipeline, error) {
	switch cfg.Source {
	case "csv":
		live := marketfeed.NewCSVSource(cfg.CSVPath, cfg.Symbols[0])
		return ingestion.NewPipeline(live, nil, marketfeed.SourceID("csv"), cfg.Symbols), nil
	default:
		creds := marketfeed.Credentials{Key: cfg.APIKey, Secret: cfg.APISecret}
		live := marketfeed.NewAlpacaStream(cfg.StreamURL, cfg.Feed, creds)
		historical := marketfeed.NewAlpacaREST(cfg.DataREST, cfg.Feed, creds)
		return ingestion.NewPipeline(live, historical, marketfeed.SourceID(cfg.Feed), cfg.Symbols), nil
	}
}
