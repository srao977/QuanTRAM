package server

import (
	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/config"
	"quantram/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StreamVolumeEvents publishes Host-produced VolumeEvents. It does not ingest
// Bars or run Volume science. A slow or cancelled client cannot block ModelHost.
func (s *Server) StreamVolumeEvents(request *quantramv1.StreamVolumeEventsRequest, stream quantramv1.ModelService_StreamVolumeEventsServer) error {
	if s.volumes == nil {
		return status.Error(codes.FailedPrecondition, "volume is not wired")
	}
	wanted, err := normalizeSymbols(request.GetSymbols())
	if err != nil {
		return err
	}

	id, events := s.volumes.SubscribeVolumeEvents(config.SubscriberQueue)
	defer s.volumes.UnsubscribeVolumeEvents(id)

	var sent uint32
	seen := make(map[string]struct{})
	for _, ev := range s.volumes.LastVolumeEvents() {
		if len(wanted) > 0 && !wanted[ev.Lineage.Symbol] {
			continue
		}
		if err := stream.Send(toProtoVolumeEvent(ev)); err != nil {
			return err
		}
		if ev.EventID != "" {
			seen[ev.EventID] = struct{}{}
		}
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
			if ev.EventID != "" {
				if _, dup := seen[ev.EventID]; dup {
					delete(seen, ev.EventID)
					continue
				}
			}
			if len(wanted) > 0 && !wanted[ev.Lineage.Symbol] {
				continue
			}
			if err := stream.Send(toProtoVolumeEvent(ev)); err != nil {
				return err
			}
			sent++
			if request.GetMaxEvents() > 0 && sent >= request.GetMaxEvents() {
				return nil
			}
		}
	}
}

// toProtoVolumeEvent maps validated domain Volume Output to the canonical
// protobuf contract. It does not recompute Volume science.
func toProtoVolumeEvent(ev domain.VolumeEvent) *quantramv1.VolumeEvent {
	out := &quantramv1.VolumeEvent{
		EventId:             ev.EventID,
		Symbol:              ev.Lineage.Symbol,
		IntervalStartUnixMs: unixMilli(ev.Lineage.IntervalStart),
		IntervalEndUnixMs:   unixMilli(ev.Lineage.IntervalEnd),
		MarketSnapshotId:    ev.Lineage.MarketSnapshotID,
		SourceTimestamp:     ev.Lineage.SourceTimestamp,
		AcceptedSequence:    uint64(ev.AcceptedSequence),
		Status:              toProtoVolumeStatus(ev.Status),
		Emitted:             ev.Status == domain.VolumeStatusAvailable,
		Reason:              ev.Reason,
		Emission:            toProtoVolumeEmission(ev),
	}
	if skip := toProtoVolumeSkip(ev.Status, ev.Reason); skip != nil {
		out.Skip = skip
	}
	return out
}

func toProtoVolumeEmission(ev domain.VolumeEvent) *quantramv1.VolumeEmission {
	return &quantramv1.VolumeEmission{
		VRaw:            toProtoVolumeQuantity(ev.VRaw),
		VN:              toProtoVolumeQuantity(ev.VN),
		V1:              toProtoVolumeQuantity(ev.V1),
		V2:              toProtoVolumeQuantity(ev.V2),
		IntervalMeanVn:  toProtoVolumeQuantity(ev.IntervalMeanVN),
		PredictedNextVN: toProtoVolumeQuantity(ev.PredictedNextVN),
		RawColor:        toProtoVolumeIndicator(ev.RawColor),
		Indicator:       toProtoVolumeIndicator(ev.Indicator),
		Transition:      toProtoVolumeTransition(ev.Transition),
		Phase:           toProtoVolumePhase(ev.Phase),
		Confidence:      toProtoVolumeConfidence(ev.Confidence),
		DomainState:     toProtoVolumeDomainState(ev.DomainState),
	}
}

func toProtoVolumeQuantity(q domain.VolumeQuantity) *quantramv1.VolumeQuantity {
	out := &quantramv1.VolumeQuantity{Status: toProtoVolumeQuantityStatus(q.Status)}
	if q.Status == domain.VolumeQtyAvailable {
		value := q.Value
		out.Value = &value
	}
	return out
}

func toProtoVolumeSkip(status, reason string) *quantramv1.VolumeSkip {
	switch status {
	case domain.VolumeStatusMaturing:
		return &quantramv1.VolumeSkip{
			Reason: quantramv1.VolumeSkipReason_VOLUME_SKIP_REASON_MATURING,
			Detail: reason,
		}
	case domain.VolumeStatusInvalid:
		return &quantramv1.VolumeSkip{
			Reason: quantramv1.VolumeSkipReason_VOLUME_SKIP_REASON_INVALID,
			Detail: reason,
		}
	case domain.VolumeStatusError:
		return &quantramv1.VolumeSkip{
			Reason: quantramv1.VolumeSkipReason_VOLUME_SKIP_REASON_ENGINE_ERROR,
			Detail: reason,
		}
	default:
		return nil
	}
}

func toProtoVolumeStatus(value string) quantramv1.VolumeStatus {
	switch value {
	case domain.VolumeStatusMaturing:
		return quantramv1.VolumeStatus_VOLUME_STATUS_MATURING
	case domain.VolumeStatusAvailable:
		return quantramv1.VolumeStatus_VOLUME_STATUS_AVAILABLE
	case domain.VolumeStatusInvalid:
		return quantramv1.VolumeStatus_VOLUME_STATUS_INVALID
	case domain.VolumeStatusError:
		return quantramv1.VolumeStatus_VOLUME_STATUS_ENGINE_ERROR
	default:
		return quantramv1.VolumeStatus_VOLUME_STATUS_UNSPECIFIED
	}
}

func toProtoVolumeQuantityStatus(value string) quantramv1.VolumeQuantityStatus {
	switch value {
	case domain.VolumeQtyInsufficient:
		return quantramv1.VolumeQuantityStatus_VOLUME_QUANTITY_STATUS_INSUFFICIENT
	case domain.VolumeQtyAvailable:
		return quantramv1.VolumeQuantityStatus_VOLUME_QUANTITY_STATUS_AVAILABLE
	case domain.VolumeQtyUndefined:
		return quantramv1.VolumeQuantityStatus_VOLUME_QUANTITY_STATUS_UNDEFINED
	default:
		return quantramv1.VolumeQuantityStatus_VOLUME_QUANTITY_STATUS_UNSPECIFIED
	}
}

func toProtoVolumeIndicator(value string) quantramv1.VolumeIndicator {
	switch value {
	case domain.VolumeColorGreen:
		return quantramv1.VolumeIndicator_VOLUME_INDICATOR_GREEN
	case domain.VolumeColorAmber:
		return quantramv1.VolumeIndicator_VOLUME_INDICATOR_AMBER
	case domain.VolumeColorRed:
		return quantramv1.VolumeIndicator_VOLUME_INDICATOR_RED
	default:
		return quantramv1.VolumeIndicator_VOLUME_INDICATOR_UNSPECIFIED
	}
}

func toProtoVolumeTransition(value string) quantramv1.VolumeTransition {
	switch value {
	case "STABLE":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_STABLE
	case "PENDING_GREEN":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_GREEN
	case "PENDING_AMBER":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_AMBER
	case "PENDING_RED":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_RED
	case "CONFIRMED_GREEN":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_CONFIRMED_GREEN
	case "CONFIRMED_AMBER":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_CONFIRMED_AMBER
	case "CONFIRMED_RED":
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_CONFIRMED_RED
	default:
		return quantramv1.VolumeTransition_VOLUME_TRANSITION_UNSPECIFIED
	}
}

func toProtoVolumePhase(value string) quantramv1.VolumePhase {
	switch value {
	case domain.VolumePhaseStationary:
		return quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_STATIONARY
	case domain.VolumePhaseIncreasingAccelerating:
		return quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_INCREASING_ACCELERATING
	case domain.VolumePhaseIncreasingDecelerating:
		return quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_INCREASING_DECELERATING
	case domain.VolumePhaseDecreasingAccelerating:
		return quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_DECREASING_ACCELERATING
	case domain.VolumePhaseDecreasingDecelerating:
		return quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_DECREASING_DECELERATING
	default:
		return quantramv1.VolumePhase_VOLUME_PHASE_UNSPECIFIED
	}
}

func toProtoVolumeConfidence(value string) quantramv1.VolumeConfidence {
	if value == domain.VolumeConfidenceHigh {
		return quantramv1.VolumeConfidence_VOLUME_CONFIDENCE_HIGH
	}
	return quantramv1.VolumeConfidence_VOLUME_CONFIDENCE_UNSPECIFIED
}

func toProtoVolumeDomainState(value string) quantramv1.VolumeDomainState {
	if value == domain.VolumeDomainCausalLocal {
		return quantramv1.VolumeDomainState_VOLUME_DOMAIN_STATE_CAUSAL_LOCAL_VOLUME
	}
	return quantramv1.VolumeDomainState_VOLUME_DOMAIN_STATE_UNSPECIFIED
}
