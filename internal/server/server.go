// Package server adapts ingestion, model, semantic, and Snapshot services to
// the public QuanTRAM gRPC contract.
package server

import (
	"context"
	"strings"
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/domain"
	"quantram/internal/ingestion"
	"quantram/internal/semantics"
	"quantram/internal/snapshot"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server exposes configured QuanTRAM components through the generated gRPC
// service interfaces.
type Server struct {
	quantramv1.UnimplementedMarketFeedServiceServer
	quantramv1.UnimplementedIngestionServiceServer
	quantramv1.UnimplementedOperationsServiceServer
	quantramv1.UnimplementedModelServiceServer
	quantramv1.UnimplementedSemanticServiceServer
	quantramv1.UnimplementedSnapshotServiceServer
	pipeline  *ingestion.Pipeline
	host      modelHealth
	events    modelEvents
	prices    priceEvents
	semantics *semantics.Dictionary
	snapshots *snapshot.Service
}

type modelHealth interface {
	Health() domain.ComponentHealth
}

type pricingHealth interface {
	PricingHealth() domain.ComponentHealth
}

type modelEvents interface {
	SubscribeEvents(buffer int) (uint64, <-chan domain.DecisionEvent)
	UnsubscribeEvents(id uint64)
	LastEvents() []domain.DecisionEvent
}

type priceEvents interface {
	SubscribePriceEvents(buffer int) (uint64, <-chan domain.PriceEvent)
	UnsubscribePriceEvents(id uint64)
	LastPriceEvents() []domain.PriceEvent
	PricingEnabled() bool
}

// New composes a gRPC Server and enables event streams only when the supplied
// model host implements their source interfaces.
func New(pipeline *ingestion.Pipeline, host modelHealth) *Server {
	s := &Server{pipeline: pipeline, host: host}
	if ev, ok := host.(modelEvents); ok {
		s.events = ev
	}
	if px, ok := host.(priceEvents); ok && px.PricingEnabled() {
		s.prices = px
	}
	return s
}

// GetFeedHealth returns the current market-feed health observation.
func (s *Server) GetFeedHealth(context.Context, *quantramv1.GetFeedHealthRequest) (*quantramv1.FeedHealth, error) {
	return toProtoHealth(s.pipeline.FeedHealth()), nil
}

// GetActiveSource returns the source identity and its current feed state.
func (s *Server) GetActiveSource(context.Context, *quantramv1.GetActiveSourceRequest) (*quantramv1.ActiveSource, error) {
	health := s.pipeline.FeedHealth()
	return &quantramv1.ActiveSource{
		SourceId: health.SourceID,
		State:    toProtoFeedState(health.State),
	}, nil
}

// StreamBars sends the current in-memory window before forwarding subscribed
// bars. The subscription is established first so arrivals during catch-up are
// retained by the pipeline channel; this bar stream does not deduplicate an
// event that overlaps the window snapshot and subscription.
func (s *Server) StreamBars(request *quantramv1.StreamBarsRequest, stream quantramv1.IngestionService_StreamBarsServer) error {
	wanted, err := normalizeSymbols(request.GetSymbols())
	if err != nil {
		return err
	}
	var id uint64
	var bars <-chan domain.Bar
	if request.GetFinalizedOnly() {
		id, bars = s.pipeline.SubscribeFinalized(0)
	} else {
		id, bars = s.pipeline.Subscribe(0)
	}
	defer s.pipeline.Unsubscribe(id)

	var sent uint32
	for _, symbol := range catchUpSymbols(wanted, request.GetSymbols(), s.pipeline.Symbols()) {
		for _, bar := range s.pipeline.Window(symbol, 0) {
			if request.GetFinalizedOnly() && !bar.IsFinal {
				continue
			}
			if err := stream.Send(toProtoBar(bar)); err != nil {
				return err
			}
			sent++
			if request.GetMaxBars() > 0 && sent >= request.GetMaxBars() {
				return nil
			}
		}
	}

	for {
		select {
		case <-stream.Context().Done():
			return status.FromContextError(stream.Context().Err()).Err()
		case bar, ok := <-bars:
			if !ok {
				return nil
			}
			if len(wanted) > 0 && !wanted[bar.Symbol] {
				continue
			}
			if err := stream.Send(toProtoBar(bar)); err != nil {
				return err
			}
			sent++
			if request.GetMaxBars() > 0 && sent >= request.GetMaxBars() {
				return nil
			}
		}
	}
}

// GetBarWindow returns the requested symbol's bounded in-memory bar window.
func (s *Server) GetBarWindow(_ context.Context, request *quantramv1.GetBarWindowRequest) (*quantramv1.BarWindow, error) {
	symbol := strings.ToUpper(strings.TrimSpace(request.GetSymbol()))
	if symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}
	bars := s.pipeline.Window(symbol, int(request.GetLimit()))
	out := make([]*quantramv1.Bar, 0, len(bars))
	for _, bar := range bars {
		out = append(out, toProtoBar(bar))
	}
	return &quantramv1.BarWindow{Symbol: symbol, Bars: out}, nil
}

// TriggerGapFill requests historical recovery for one normalized symbol.
func (s *Server) TriggerGapFill(ctx context.Context, request *quantramv1.TriggerGapFillRequest) (*quantramv1.GapFillResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(request.GetSymbol()))
	if symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}
	from := unixMs(request.GetFromUnixMs())
	to := unixMs(request.GetToUnixMs())
	fetched, injected, deduped, err := s.pipeline.GapFill(ctx, symbol, from, to)
	result := &quantramv1.GapFillResult{
		Symbol:           symbol,
		BarsFetched:      uint32(fetched),
		BarsInjected:     uint32(injected),
		BarsDeduplicated: uint32(deduped),
	}
	if err != nil {
		result.Message = err.Error()
		return result, status.Errorf(codes.Internal, "gap fill: %v", err)
	}
	result.Message = "gap fill completed"
	return result, nil
}

// GetHealth aggregates ingestion, model, and pricing component health.
func (s *Server) GetHealth(context.Context, *quantramv1.GetHealthRequest) (*quantramv1.HealthReport, error) {
	report := s.pipeline.Health()
	if s.host != nil {
		model := s.host.Health()
		report.Components = append(report.Components, model)
		if model.State == domain.ComponentUnavailable {
			report.State = domain.ComponentUnavailable
		} else if model.State == domain.ComponentDegraded && report.State == domain.ComponentHealthy {
			report.State = domain.ComponentDegraded
		}
	} else {
		report.Components = append(report.Components, domain.ComponentHealth{
			Name: "model", State: domain.ComponentHealthy, Detail: "off",
		})
	}
	pricing := domain.ComponentHealth{Name: "pricing", State: domain.ComponentHealthy, Detail: "off"}
	if ph, ok := s.host.(pricingHealth); ok {
		pricing = ph.PricingHealth()
	}
	report.Components = append(report.Components, pricing)
	applyPricingToReport(&report, pricing)
	out := &quantramv1.HealthReport{State: toProtoComponent(report.State)}
	for _, component := range report.Components {
		out.Components = append(out.Components, &quantramv1.ComponentHealth{
			Name:   component.Name,
			State:  toProtoComponent(component.State),
			Detail: component.Detail,
		})
	}
	return out, nil
}

// GetReadiness returns the pipeline's observation and inference gates.
func (s *Server) GetReadiness(context.Context, *quantramv1.GetReadinessRequest) (*quantramv1.ReadinessReport, error) {
	ready := s.pipeline.Readiness()
	return &quantramv1.ReadinessReport{
		Ready:   ready.Ready,
		Observe: ready.Observe,
		Infer:   ready.Infer,
		Message: ready.Message,
	}, nil
}

func catchUpSymbols(wanted map[string]bool, requested, configured []string) []string {
	if len(wanted) > 0 {
		out := make([]string, 0, len(wanted))
		for _, raw := range requested {
			symbol := strings.ToUpper(strings.TrimSpace(raw))
			if symbol != "" {
				out = append(out, symbol)
			}
		}
		return out
	}
	return configured
}

func normalizeSymbols(symbols []string) (map[string]bool, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	wanted := make(map[string]bool, len(symbols))
	for _, raw := range symbols {
		symbol := strings.ToUpper(strings.TrimSpace(raw))
		if symbol == "" {
			return nil, status.Error(codes.InvalidArgument, "symbol must not be empty")
		}
		wanted[symbol] = true
	}
	return wanted, nil
}

func unixMs(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func toProtoBar(bar domain.Bar) *quantramv1.Bar {
	return &quantramv1.Bar{
		Symbol:              bar.Symbol,
		InstrumentId:        bar.InstrumentID,
		InstrumentType:      toProtoInstrument(bar.InstrumentType),
		Tradable:            bar.Tradable,
		Interval:            bar.Interval,
		IntervalStartUnixMs: bar.IntervalStart.UnixMilli(),
		IntervalEndUnixMs:   bar.IntervalEnd.UnixMilli(),
		Open:                bar.Open,
		High:                bar.High,
		Low:                 bar.Low,
		Close:               bar.Close,
		Volume:              bar.Volume,
		EventCount:          bar.EventCount,
		SourceTimestamp:     bar.SourceTimestamp,
		ReceiptUnixMs:       bar.ReceiptTime.UnixMilli(),
		Source:              bar.Source,
		QualityStatus:       toProtoQuality(bar.QualityStatus),
		IsFinal:             bar.IsFinal,
		IsBackfilled:        bar.IsBackfilled,
		SourceTransition:    bar.SourceTransition,
		DataAgeMs:           bar.DataAge(time.Now().UTC()).Milliseconds(),
		MarketSnapshotId:    bar.MarketSnapshotID,
	}
}

func toProtoHealth(health domain.FeedHealth) *quantramv1.FeedHealth {
	var lastMsg int64
	if !health.LastMessage.IsZero() {
		lastMsg = health.LastMessage.UnixMilli()
	}
	return &quantramv1.FeedHealth{
		SourceId:                     health.SourceID,
		State:                        toProtoFeedState(health.State),
		LastMessageUnixMs:            lastMsg,
		LastPongRttMs:                health.LastPongRTT.Milliseconds(),
		ConsecutiveHeartbeatFailures: health.ConsecutiveHeartbeatFailures,
		LastError:                    health.LastError,
		SubscribedSymbols:            health.SubscribedSymbols,
	}
}

func toProtoInstrument(value domain.InstrumentType) quantramv1.InstrumentType {
	switch value {
	case domain.InstrumentStock:
		return quantramv1.InstrumentType_INSTRUMENT_TYPE_STOCK
	case domain.InstrumentETF:
		return quantramv1.InstrumentType_INSTRUMENT_TYPE_ETF
	case domain.InstrumentIndex:
		return quantramv1.InstrumentType_INSTRUMENT_TYPE_INDEX
	default:
		return quantramv1.InstrumentType_INSTRUMENT_TYPE_UNSPECIFIED
	}
}

func toProtoQuality(value domain.QualityStatus) quantramv1.QualityStatus {
	switch value {
	case domain.QualityComplete:
		return quantramv1.QualityStatus_QUALITY_STATUS_COMPLETE
	case domain.QualityDegraded:
		return quantramv1.QualityStatus_QUALITY_STATUS_DEGRADED
	case domain.QualityStale:
		return quantramv1.QualityStatus_QUALITY_STATUS_STALE
	case domain.QualityPartial:
		return quantramv1.QualityStatus_QUALITY_STATUS_PARTIAL
	case domain.QualityReconstructed:
		return quantramv1.QualityStatus_QUALITY_STATUS_RECONSTRUCTED
	case domain.QualityInvalid:
		return quantramv1.QualityStatus_QUALITY_STATUS_INVALID
	default:
		return quantramv1.QualityStatus_QUALITY_STATUS_UNSPECIFIED
	}
}

func toProtoFeedState(value domain.FeedState) quantramv1.FeedState {
	switch value {
	case domain.FeedHealthy:
		return quantramv1.FeedState_FEED_STATE_HEALTHY
	case domain.FeedDegraded:
		return quantramv1.FeedState_FEED_STATE_DEGRADED
	case domain.FeedFailed:
		return quantramv1.FeedState_FEED_STATE_FAILED
	case domain.FeedRecovering:
		return quantramv1.FeedState_FEED_STATE_RECOVERING
	default:
		return quantramv1.FeedState_FEED_STATE_UNSPECIFIED
	}
}

func applyPricingToReport(report *domain.HealthReport, pricing domain.ComponentHealth) {
	switch pricing.State {
	case domain.ComponentUnavailable:
		if report.State == domain.ComponentHealthy {
			report.State = domain.ComponentDegraded
		}
	case domain.ComponentDegraded:
		switch pricing.Detail {
		case "discontinuous", "error":
			if report.State == domain.ComponentHealthy {
				report.State = domain.ComponentDegraded
			}
		}
	}
}

func toProtoComponent(value domain.ComponentState) quantramv1.ComponentState {
	switch value {
	case domain.ComponentHealthy:
		return quantramv1.ComponentState_COMPONENT_STATE_HEALTHY
	case domain.ComponentDegraded:
		return quantramv1.ComponentState_COMPONENT_STATE_DEGRADED
	case domain.ComponentUnavailable:
		return quantramv1.ComponentState_COMPONENT_STATE_UNAVAILABLE
	default:
		return quantramv1.ComponentState_COMPONENT_STATE_UNSPECIFIED
	}
}
