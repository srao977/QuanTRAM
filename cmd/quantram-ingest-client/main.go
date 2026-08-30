package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	quantramv1 "quantram/gen/quantram/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	address := flag.String("address", "localhost:50051", "QuanTRAM gRPC address")
	operation := flag.String("operation", "stream", "operation: stream, health, ready, source, window, gapfill")
	symbolsFlag := flag.String("symbols", "AAPL", "comma-separated symbols")
	maxBars := flag.Uint("max-bars", 5, "maximum bars to receive; 0 streams until timeout")
	timeout := flag.Duration("timeout", 45*time.Second, "request timeout")
	limit := flag.Uint("limit", 8, "window size for window operation")
	flag.Parse()

	connection, err := grpc.NewClient(*address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("create gRPC client: %v", err)
	}
	defer connection.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	symbols := splitSymbols(*symbolsFlag)
	feed := quantramv1.NewMarketFeedServiceClient(connection)
	ingest := quantramv1.NewIngestionServiceClient(connection)
	ops := quantramv1.NewOperationsServiceClient(connection)

	switch *operation {
	case "health":
		report, err := ops.GetHealth(ctx, &quantramv1.GetHealthRequest{})
		if err != nil {
			log.Fatalf("health: %v", err)
		}
		fmt.Printf("state=%s\n", report.GetState())
		for _, component := range report.GetComponents() {
			fmt.Printf("component=%s state=%s detail=%q\n", component.GetName(), component.GetState(), component.GetDetail())
		}
	case "ready":
		report, err := ops.GetReadiness(ctx, &quantramv1.GetReadinessRequest{})
		if err != nil {
			log.Fatalf("ready: %v", err)
		}
		fmt.Printf("ready=%t observe=%t infer=%t message=%q\n", report.GetReady(), report.GetObserve(), report.GetInfer(), report.GetMessage())
	case "source":
		health, err := feed.GetFeedHealth(ctx, &quantramv1.GetFeedHealthRequest{})
		if err != nil {
			log.Fatalf("feed health: %v", err)
		}
		active, err := feed.GetActiveSource(ctx, &quantramv1.GetActiveSourceRequest{})
		if err != nil {
			log.Fatalf("active source: %v", err)
		}
		fmt.Printf("source=%s state=%s symbols=%s last_error=%q\n",
			active.GetSourceId(), health.GetState(), strings.Join(health.GetSubscribedSymbols(), ","), health.GetLastError())
	case "window":
		if len(symbols) == 0 {
			log.Fatal("symbols is required")
		}
		window, err := ingest.GetBarWindow(ctx, &quantramv1.GetBarWindowRequest{Symbol: symbols[0], Limit: uint32(*limit)})
		if err != nil {
			log.Fatalf("window: %v", err)
		}
		fmt.Printf("symbol=%s bars=%d\n", window.GetSymbol(), len(window.GetBars()))
		for _, bar := range window.GetBars() {
			printBar(bar)
		}
	case "gapfill":
		if len(symbols) == 0 {
			log.Fatal("symbols is required")
		}
		result, err := ingest.TriggerGapFill(ctx, &quantramv1.TriggerGapFillRequest{Symbol: symbols[0]})
		if err != nil {
			log.Fatalf("gapfill: %v", err)
		}
		fmt.Printf("symbol=%s fetched=%d injected=%d deduplicated=%d message=%q\n",
			result.GetSymbol(), result.GetBarsFetched(), result.GetBarsInjected(), result.GetBarsDeduplicated(), result.GetMessage())
	case "stream":
		stream, err := ingest.StreamBars(ctx, &quantramv1.StreamBarsRequest{
			Symbols: symbols,
			MaxBars: uint32(*maxBars),
		})
		if err != nil {
			log.Fatalf("stream bars: %v", err)
		}
		count := 0
		for {
			bar, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				log.Fatalf("receive bar: %v", err)
			}
			printBar(bar)
			count++
		}
		fmt.Printf("received=%d\n", count)
	default:
		fmt.Fprintf(os.Stderr, "unsupported operation %q\n", *operation)
		os.Exit(2)
	}
}

func printBar(bar *quantramv1.Bar) {
	fmt.Printf("symbol=%s source=%s ts=%s o=%.4f h=%.4f l=%.4f c=%.4f v=%d final=%t backfill=%t quality=%s snapshot=%s\n",
		bar.GetSymbol(),
		bar.GetSource(),
		bar.GetSourceTimestamp(),
		bar.GetOpen(),
		bar.GetHigh(),
		bar.GetLow(),
		bar.GetClose(),
		bar.GetVolume(),
		bar.GetIsFinal(),
		bar.GetIsBackfilled(),
		bar.GetQualityStatus(),
		trimID(bar.GetMarketSnapshotId()),
	)
}

func trimID(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func splitSymbols(value string) []string {
	parts := strings.Split(value, ",")
	symbols := make([]string, 0, len(parts))
	for _, part := range parts {
		symbol := strings.ToUpper(strings.TrimSpace(part))
		if symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}
