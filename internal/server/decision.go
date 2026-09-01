package server

import (
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/config"
	"quantram/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) StreamDecisions(request *quantramv1.StreamDecisionsRequest, stream quantramv1.ModelService_StreamDecisionsServer) error {
	if s.events == nil {
		if s.host != nil {
			return status.Error(codes.Unavailable, "model unavailable")
		}
		return status.Error(codes.FailedPrecondition, "model is off")
	}
	wanted, err := normalizeSymbols(request.GetSymbols())
	if err != nil {
		return err
	}

	id, events := s.events.SubscribeEvents(config.SubscriberQueue)
	defer s.events.UnsubscribeEvents(id)

	var sent uint32
	seen := make(map[string]struct{})
	for _, ev := range s.events.LastEvents() {
		if len(wanted) > 0 && !wanted[ev.Symbol] {
			continue
		}
		if err := stream.Send(toProtoDecisionEvent(ev)); err != nil {
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
			if err := stream.Send(toProtoDecisionEvent(ev)); err != nil {
				return err
			}
			sent++
			if request.GetMaxEvents() > 0 && sent >= request.GetMaxEvents() {
				return nil
			}
		}
	}
}

func toProtoDecisionEvent(ev domain.DecisionEvent) *quantramv1.DecisionEvent {
	out := &quantramv1.DecisionEvent{
		EventId:             ev.EventID,
		SignalId:            ev.SignalID,
		DecisionId:          ev.DecisionID,
		Symbol:              ev.Symbol,
		IntervalStartUnixMs: unixMilli(ev.IntervalStart),
		MarketSnapshotId:    ev.MarketSnapshotID,
		SourceTimestamp:     ev.SourceTimestamp,
		AcceptedSequence:    uint64(ev.AcceptedSequence),
		ReceivedAtUnixMs:    unixMilli(ev.ReceivedAt),
		CompletedAtUnixMs:   unixMilli(ev.CompletedAt),
		LatencyMs:           ev.Latency.Milliseconds(),
		ModelVersion:        ev.ModelVersion,
		SchemaVersion:       ev.SchemaVersion,
		PreStateHash:        ev.PreStateHash,
		PostStateHash:       ev.PostStateHash,
	}
	switch {
	case ev.IsDecision():
		out.Outcome = &quantramv1.DecisionEvent_Decision{Decision: toProtoDecision(*ev.Decision)}
	case ev.IsSkip():
		out.Outcome = &quantramv1.DecisionEvent_Skip{Skip: toProtoSkip(*ev.Skip)}
	}
	return out
}

func toProtoDecision(d domain.Decision) *quantramv1.Decision {
	return &quantramv1.Decision{
		Side:                 toProtoSide(d.Side),
		Confidence:           d.Confidence,
		H:                    int32(d.H),
		QG:                   d.QG,
		QS:                   d.QS,
		QR:                   d.QR,
		PathDirection:        toProtoPath(d.PathDirection),
		ModelStatus:          toProtoModelStatus(d.ModelStatus),
		EmitterPositionState: toProtoEmitter(d.EmitterPosition),
		RulePath:             d.RulePath,
		Strength:             d.Strength,
		Coherence:            d.Coherence,
		Persistence:          d.Persistence,
		Uncertainty:          d.Uncertainty,
		Reversal:             d.Reversal,
		TerminalDisplacement: d.TerminalDisplacement,
	}
}

func toProtoSkip(s domain.Skip) *quantramv1.Skip {
	return &quantramv1.Skip{
		Reason:      toProtoSkipReason(s.Reason),
		Detail:      s.Detail,
		ModelStatus: toProtoModelStatus(s.ModelStatus),
	}
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func toProtoSide(value domain.Side) quantramv1.Side {
	switch value {
	case domain.SideBuy:
		return quantramv1.Side_SIDE_BUY
	case domain.SideSell:
		return quantramv1.Side_SIDE_SELL
	case domain.SideHold:
		return quantramv1.Side_SIDE_HOLD
	default:
		return quantramv1.Side_SIDE_UNSPECIFIED
	}
}

func toProtoPath(value domain.PathDirection) quantramv1.PathDirection {
	switch value {
	case domain.PathUpward:
		return quantramv1.PathDirection_PATH_DIRECTION_UPWARD
	case domain.PathDownward:
		return quantramv1.PathDirection_PATH_DIRECTION_DOWNWARD
	case domain.PathFlat:
		return quantramv1.PathDirection_PATH_DIRECTION_FLAT
	default:
		return quantramv1.PathDirection_PATH_DIRECTION_UNSPECIFIED
	}
}

func toProtoEmitter(value domain.EmitterPosition) quantramv1.EmitterPosition {
	switch value {
	case domain.EmitterFlat:
		return quantramv1.EmitterPosition_EMITTER_POSITION_FLAT
	case domain.EmitterLong:
		return quantramv1.EmitterPosition_EMITTER_POSITION_LONG
	case domain.EmitterShort:
		return quantramv1.EmitterPosition_EMITTER_POSITION_SHORT
	default:
		return quantramv1.EmitterPosition_EMITTER_POSITION_UNSPECIFIED
	}
}

func toProtoModelStatus(value domain.ModelStatus) quantramv1.ModelStatus {
	switch value {
	case domain.StatusInitializing:
		return quantramv1.ModelStatus_MODEL_STATUS_INITIALIZING
	case domain.StatusActionable:
		return quantramv1.ModelStatus_MODEL_STATUS_ACTIONABLE
	default:
		return quantramv1.ModelStatus_MODEL_STATUS_UNSPECIFIED
	}
}

func toProtoSkipReason(value domain.SkipReason) quantramv1.SkipReason {
	switch value {
	case domain.SkipInferOff:
		return quantramv1.SkipReason_SKIP_REASON_INFER_OFF
	case domain.SkipNotModelEligible:
		return quantramv1.SkipReason_SKIP_REASON_NOT_MODEL_ELIGIBLE
	case domain.SkipInitializing:
		return quantramv1.SkipReason_SKIP_REASON_INITIALIZING
	case domain.SkipDuplicateOrRegression:
		return quantramv1.SkipReason_SKIP_REASON_DUPLICATE_OR_REGRESSION
	case domain.SkipInputGap:
		return quantramv1.SkipReason_SKIP_REASON_INPUT_GAP
	case domain.SkipQueueOverflow:
		return quantramv1.SkipReason_SKIP_REASON_QUEUE_OVERFLOW
	case domain.SkipTimeout:
		return quantramv1.SkipReason_SKIP_REASON_TIMEOUT
	case domain.SkipInvalidInput:
		return quantramv1.SkipReason_SKIP_REASON_INVALID_INPUT
	case domain.SkipEngineError:
		return quantramv1.SkipReason_SKIP_REASON_ENGINE_ERROR
	case domain.SkipEnginePanic:
		return quantramv1.SkipReason_SKIP_REASON_ENGINE_PANIC
	case domain.SkipStateDiscontinuous:
		return quantramv1.SkipReason_SKIP_REASON_STATE_DISCONTINUOUS
	default:
		return quantramv1.SkipReason_SKIP_REASON_UNSPECIFIED
	}
}
