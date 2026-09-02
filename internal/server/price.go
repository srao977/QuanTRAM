package server

import (
	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/config"
	"quantram/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) StreamPriceEvents(request *quantramv1.StreamPriceEventsRequest, stream quantramv1.ModelService_StreamPriceEventsServer) error {
	if s.prices == nil {
		if s.host != nil {
			if ph, ok := s.host.(pricingHealth); ok {
				h := ph.PricingHealth()
				if h.State == domain.ComponentUnavailable {
					return status.Error(codes.Unavailable, "pricing unavailable")
				}
			}
		}
		return status.Error(codes.FailedPrecondition, "pricing is off")
	}
	wanted, err := normalizeSymbols(request.GetSymbols())
	if err != nil {
		return err
	}

	id, events := s.prices.SubscribePriceEvents(config.SubscriberQueue)
	defer s.prices.UnsubscribePriceEvents(id)

	var sent uint32
	seen := make(map[string]struct{})
	for _, ev := range s.prices.LastPriceEvents() {
		if len(wanted) > 0 && !wanted[ev.Symbol] {
			continue
		}
		if err := stream.Send(toProtoPriceEvent(ev)); err != nil {
			return err
		}
		seen[ev.EventID] = struct{}{}
		sent++
		if request.GetMaxEvents() > 0 && sent >= request.GetMaxEvents() {
			return nil
		}
	}

	for {
		select {
		case <-stream.Context().Done():
			return status.FromContextError(stream.Context().Err()).Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if _, dup := seen[ev.EventID]; dup {
				delete(seen, ev.EventID)
				continue
			}
			if len(wanted) > 0 && !wanted[ev.Symbol] {
				continue
			}
			if err := stream.Send(toProtoPriceEvent(ev)); err != nil {
				return err
			}
			sent++
			if request.GetMaxEvents() > 0 && sent >= request.GetMaxEvents() {
				return nil
			}
		}
	}
}

func toProtoPriceEvent(ev domain.PriceEvent) *quantramv1.PriceEvent {
	out := &quantramv1.PriceEvent{
		EventId:             ev.EventID,
		Symbol:              ev.Symbol,
		IntervalStartUnixMs: unixMilli(ev.IntervalStart),
		MarketSnapshotId:    ev.MarketSnapshotID,
		SourceTimestamp:     ev.SourceTimestamp,
		AcceptedSequence:    uint64(ev.AcceptedSequence),
		LatencyMs:           ev.Latency.Milliseconds(),
		Status:              toProtoPricingStatus(ev.Status),
		Emitted:             ev.Emitted,
		DomainExit:          ev.DomainExit,
		RkSuccess:           ev.RKSuccess,
	}
	if ev.Emission != nil {
		out.Emission = toProtoPriceEmission(*ev.Emission)
	}
	if ev.Skip != nil {
		out.Skip = toProtoPricingSkip(*ev.Skip)
	}
	if ev.Cockpit != nil {
		out.Cockpit = toProtoPriceCockpit(*ev.Cockpit)
	}
	return out
}

func toProtoPriceEmission(em domain.PriceEmission) *quantramv1.PriceEmission {
	return &quantramv1.PriceEmission{
		Color:                     em.Color,
		TrajectoryPhase:           em.TrajectoryPhase,
		TurningTendency:           em.TurningTendency,
		ConfidenceState:           em.ConfidenceState,
		DomainState:               em.DomainState,
		StabilityState:            em.StabilityState,
		CurrentDirection:          em.CurrentDirection,
		ProjectedDirection:        em.ProjectedDirection,
		ReasonCodes:               append([]string(nil), em.ReasonCodes...),
		RkSuccess:                 em.RKSuccess,
		ConditionNumber:           em.ConditionNumber,
		MaxRealEigenvalue:         em.MaxRealEigenvalue,
		PerturbationAmplification: em.PerturbationAmplification,
	}
}

func toProtoPriceCockpit(c domain.PriceCockpit) *quantramv1.PriceCockpit {
	return &quantramv1.PriceCockpit{
		CockpitColor:         c.CockpitColor,
		RefinedInternalState: c.RefinedInternalState,
		PersistenceState:     c.PersistenceState,
		TurnCandidate:        c.TurnCandidate,
		DomainState:          c.DomainState,
		ConfidenceState:      c.ConfidenceState,
	}
}

func toProtoPricingSkip(s domain.PricingSkip) *quantramv1.PricingSkip {
	return &quantramv1.PricingSkip{
		Reason: toProtoPricingSkipReason(s.Reason),
		Detail: s.Detail,
	}
}

func toProtoPricingStatus(value domain.PricingStatus) quantramv1.PricingStatus {
	switch value {
	case domain.PricingStatusWarmupDerivative:
		return quantramv1.PricingStatus_PRICING_STATUS_WARMUP_DERIVATIVE
	case domain.PricingStatusWarmupF4:
		return quantramv1.PricingStatus_PRICING_STATUS_WARMUP_F4
	case domain.PricingStatusF4Unavailable:
		return quantramv1.PricingStatus_PRICING_STATUS_F4_UNAVAILABLE
	case domain.PricingStatusEmitted:
		return quantramv1.PricingStatus_PRICING_STATUS_EMITTED
	case domain.PricingStatusProjectionFailure:
		return quantramv1.PricingStatus_PRICING_STATUS_PROJECTION_FAILURE
	default:
		return quantramv1.PricingStatus_PRICING_STATUS_UNSPECIFIED
	}
}

func toProtoPricingSkipReason(value domain.PricingSkipReason) quantramv1.PricingSkipReason {
	switch value {
	case domain.PricingSkipWarmupDerivative:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_WARMUP_DERIVATIVE
	case domain.PricingSkipWarmupF4:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_WARMUP_F4
	case domain.PricingSkipF4Unavailable:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_F4_UNAVAILABLE
	case domain.PricingSkipProjectionFail:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_PROJECTION_FAILURE
	case domain.PricingSkipUnstable:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_NUMERICALLY_UNSTABLE
	case domain.PricingSkipInvalidInput:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_INVALID_INPUT
	case domain.PricingSkipTimeout:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_TIMEOUT
	case domain.PricingSkipEngineError:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_ENGINE_ERROR
	case domain.PricingSkipEnginePanic:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_ENGINE_PANIC
	case domain.PricingSkipTimeTerm:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_TIME_TERM
	default:
		return quantramv1.PricingSkipReason_PRICING_SKIP_REASON_UNSPECIFIED
	}
}
