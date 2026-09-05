package server

import (
	"strings"
	"testing"
	"time"

	quantramv1 "quantram/gen/quantram/v1"
	"quantram/internal/domain"
	"quantram/internal/ingestion"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestC01C02VolumeProtoCompiles(t *testing.T) {
	_ = (*quantramv1.VolumeEvent)(nil)
	_ = quantramv1.VolumeStatus_VOLUME_STATUS_AVAILABLE
}

func TestC03VolumeStatusEnum(t *testing.T) {
	want := map[quantramv1.VolumeStatus]int32{
		quantramv1.VolumeStatus_VOLUME_STATUS_UNSPECIFIED:  0,
		quantramv1.VolumeStatus_VOLUME_STATUS_MATURING:     1,
		quantramv1.VolumeStatus_VOLUME_STATUS_AVAILABLE:    2,
		quantramv1.VolumeStatus_VOLUME_STATUS_INVALID:      3,
		quantramv1.VolumeStatus_VOLUME_STATUS_ENGINE_ERROR: 4,
	}
	for k, n := range want {
		if int32(k) != n {
			t.Fatalf("%s = %d want %d", k, k, n)
		}
	}
}

func TestC04IndicatorEnum(t *testing.T) {
	want := map[quantramv1.VolumeIndicator]int32{
		quantramv1.VolumeIndicator_VOLUME_INDICATOR_UNSPECIFIED: 0,
		quantramv1.VolumeIndicator_VOLUME_INDICATOR_GREEN:       1,
		quantramv1.VolumeIndicator_VOLUME_INDICATOR_AMBER:       2,
		quantramv1.VolumeIndicator_VOLUME_INDICATOR_RED:         3,
	}
	for k, n := range want {
		if int32(k) != n {
			t.Fatalf("%s = %d want %d", k, k, n)
		}
	}
}

func TestC05RawColorSharesIndicatorEnum(t *testing.T) {
	fd := (*quantramv1.VolumeEmission)(nil).ProtoReflect().Descriptor().Fields().ByName("raw_color")
	if fd.Enum().FullName() != "quantram.v1.VolumeIndicator" {
		t.Fatalf("raw_color enum %s", fd.Enum().FullName())
	}
}

func TestC06ConfirmationTransitionEnum(t *testing.T) {
	want := map[quantramv1.VolumeTransition]int32{
		quantramv1.VolumeTransition_VOLUME_TRANSITION_UNSPECIFIED:     0,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_STABLE:          1,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_GREEN:   2,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_AMBER:   3,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_RED:     4,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_CONFIRMED_GREEN: 5,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_CONFIRMED_AMBER: 6,
		quantramv1.VolumeTransition_VOLUME_TRANSITION_CONFIRMED_RED:   7,
	}
	for k, n := range want {
		if int32(k) != n {
			t.Fatalf("%s = %d want %d", k, k, n)
		}
	}
}

func TestC07PhaseEnum(t *testing.T) {
	want := map[quantramv1.VolumePhase]int32{
		quantramv1.VolumePhase_VOLUME_PHASE_UNSPECIFIED:                       0,
		quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_STATIONARY:              1,
		quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_INCREASING_ACCELERATING: 2,
		quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_INCREASING_DECELERATING: 3,
		quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_DECREASING_ACCELERATING: 4,
		quantramv1.VolumePhase_VOLUME_PHASE_ACTIVITY_DECREASING_DECELERATING: 5,
	}
	for k, n := range want {
		if int32(k) != n {
			t.Fatalf("%s = %d want %d", k, k, n)
		}
	}
}

func TestC08ScientificQuantitiesPresent(t *testing.T) {
	fields := (*quantramv1.VolumeEmission)(nil).ProtoReflect().Descriptor().Fields()
	for _, name := range []protoreflect.Name{"v_raw", "v_n", "v1", "v2", "interval_mean_vn", "predicted_next_v_n"} {
		if fields.ByName(name) == nil {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestC09C12UnavailableVsZero(t *testing.T) {
	start := time.Date(2023, 3, 30, 11, 16, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	zeroVN := domain.VolumeEvent{
		Lineage: domain.VolumeLineage{
			Symbol:           "SPY",
			MarketSnapshotID: "ms-zero",
			IntervalStart:    start,
			IntervalEnd:      end,
			SourceTimestamp:  "2023-03-30 07:16:00",
		},
		Status: domain.VolumeStatusAvailable,
		Reason: "AVAILABLE",
		VRaw:   domain.VolumeQuantity{Value: 0, Status: domain.VolumeQtyAvailable},
		VN:     domain.VolumeQuantity{Value: 0, Status: domain.VolumeQtyAvailable},
		V1:     domain.VolumeQuantity{Value: 0, Status: domain.VolumeQtyAvailable},
		V2:     domain.VolumeQuantity{Value: 1.5, Status: domain.VolumeQtyAvailable},
		IntervalMeanVN: domain.VolumeQuantity{
			Value:  1.0,
			Status: domain.VolumeQtyAvailable,
		},
		PredictedNextVN: domain.VolumeQuantity{Value: 0, Status: domain.VolumeQtyAvailable},
		RawColor:        domain.VolumeColorAmber,
		Indicator:       domain.VolumeColorAmber,
		Transition:      "STABLE",
		Phase:           domain.VolumePhaseStationary,
		Confidence:      domain.VolumeConfidenceHigh,
		DomainState:     domain.VolumeDomainCausalLocal,
	}
	gotZero := toProtoVolumeEvent(zeroVN)
	if gotZero.GetEmission().GetVN().Value == nil {
		t.Fatal("C10: available V_N=0 must set optional value")
	}
	if gotZero.GetEmission().GetVN().GetValue() != 0 {
		t.Fatalf("C10: present zero got %v", gotZero.GetEmission().GetVN().GetValue())
	}
	if gotZero.GetEmission().GetVRaw().Value == nil || gotZero.GetEmission().GetVRaw().GetValue() != 0 {
		t.Fatal("present zero V_RAW")
	}

	missing := domain.VolumeEvent{
		Lineage: zeroVN.Lineage,
		Status:  domain.VolumeStatusMaturing,
		Reason:  "MATURING_FEATURES",
		VRaw:    domain.VolumeQuantity{Value: 12, Status: domain.VolumeQtyAvailable},
		VN:      domain.VolumeQuantity{Status: domain.VolumeQtyInsufficient},
		V1:      domain.VolumeQuantity{Status: domain.VolumeQtyInsufficient},
		V2:      domain.VolumeQuantity{Status: domain.VolumeQtyUndefined},
		IntervalMeanVN: domain.VolumeQuantity{
			Status: domain.VolumeQtyInsufficient,
		},
		PredictedNextVN: domain.VolumeQuantity{Status: domain.VolumeQtyInsufficient},
	}
	gotMissing := toProtoVolumeEvent(missing)
	vn := gotMissing.GetEmission().GetVN()
	if vn.Value != nil {
		t.Fatal("C11: unavailable V_N must omit value")
	}
	if vn.GetStatus() != quantramv1.VolumeQuantityStatus_VOLUME_QUANTITY_STATUS_INSUFFICIENT {
		t.Fatalf("C11 status %s", vn.GetStatus())
	}
	if gotMissing.GetEmission().GetV1().Value != nil {
		t.Fatal("C12: unavailable V1 must omit value")
	}
	if gotMissing.GetEmission().GetV2().GetStatus() != quantramv1.VolumeQuantityStatus_VOLUME_QUANTITY_STATUS_UNDEFINED {
		t.Fatal("C12: undefined V2")
	}
	if proto.Equal(gotZero.GetEmission().GetVN(), gotMissing.GetEmission().GetVN()) {
		t.Fatal("zero and unavailable V_N must not encode identically")
	}
}

func TestC13C15LineagePreserved(t *testing.T) {
	start := time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)
	end := start.Add(time.Minute)
	ev := toProtoVolumeEvent(domain.VolumeEvent{
		Lineage: domain.VolumeLineage{
			Symbol:           "SPY",
			MarketSnapshotID: "snap-1",
			IntervalStart:    start,
			IntervalEnd:      end,
			SourceTimestamp:  "exact-provider-text",
		},
		Status: domain.VolumeStatusMaturing,
		Reason: "MATURING_FEATURES",
		VRaw:   domain.VolumeQuantity{Value: 100, Status: domain.VolumeQtyAvailable},
	})
	if ev.GetMarketSnapshotId() != "snap-1" {
		t.Fatalf("C13 market_snapshot_id %q", ev.GetMarketSnapshotId())
	}
	if ev.GetIntervalStartUnixMs() != start.UnixMilli() {
		t.Fatalf("C14 interval_start %d", ev.GetIntervalStartUnixMs())
	}
	if ev.GetIntervalEndUnixMs() != end.UnixMilli() {
		t.Fatalf("interval_end %d", ev.GetIntervalEndUnixMs())
	}
	if ev.GetSourceTimestamp() != "exact-provider-text" {
		t.Fatalf("C15 source_timestamp %q", ev.GetSourceTimestamp())
	}
}

func TestC16C17StatusesDistinct(t *testing.T) {
	maturing := toProtoVolumeEvent(domain.VolumeEvent{Status: domain.VolumeStatusMaturing, Reason: "MATURING_FEATURES"})
	invalid := toProtoVolumeEvent(domain.VolumeEvent{Status: domain.VolumeStatusInvalid, Reason: "UNDEFINED_VN"})
	errEv := toProtoVolumeEvent(domain.VolumeEvent{Status: domain.VolumeStatusError, Reason: "ENTITY_MISMATCH"})
	available := toProtoVolumeEvent(domain.VolumeEvent{Status: domain.VolumeStatusAvailable, Reason: "AVAILABLE"})

	if maturing.GetStatus() == errEv.GetStatus() {
		t.Fatal("C16 MATURING must not equal ENGINE_ERROR")
	}
	if invalid.GetStatus() == errEv.GetStatus() {
		t.Fatal("C17 INVALID must not equal ENGINE_ERROR")
	}
	if maturing.GetSkip().GetReason() != quantramv1.VolumeSkipReason_VOLUME_SKIP_REASON_MATURING {
		t.Fatal("MATURING skip")
	}
	if invalid.GetSkip().GetReason() != quantramv1.VolumeSkipReason_VOLUME_SKIP_REASON_INVALID {
		t.Fatal("INVALID skip")
	}
	if errEv.GetSkip().GetReason() != quantramv1.VolumeSkipReason_VOLUME_SKIP_REASON_ENGINE_ERROR {
		t.Fatal("ENGINE_ERROR skip")
	}
	if available.GetSkip() != nil || !available.GetEmitted() {
		t.Fatal("AVAILABLE must emit and omit skip")
	}
	if maturing.GetEmitted() || invalid.GetEmitted() || errEv.GetEmitted() {
		t.Fatal("non-AVAILABLE must not set emitted")
	}
}

func TestC18IndicatorIsNotTrading(t *testing.T) {
	for _, name := range quantramv1.VolumeIndicator_name {
		upper := strings.ToUpper(name)
		for _, tok := range []string{"BUY", "SELL", "HOLD"} {
			if strings.Contains(upper, tok) {
				t.Fatalf("C18 Indicator must not encode %s: %s", tok, name)
			}
		}
	}
}

func TestC19C21VolumeContractIsolation(t *testing.T) {
	forbidden := []string{"price", "adaptive", "decision", "rk", "rk45", "expm", "ode", "trajectory", "eigen", "buy", "sell", "hold", "order"}
	check := func(desc protoreflect.MessageDescriptor) {
		fields := desc.Fields()
		for i := 0; i < fields.Len(); i++ {
			parts := strings.Split(strings.ToLower(string(fields.Get(i).Name())), "_")
			for _, tok := range forbidden {
				for _, part := range parts {
					if part == tok {
						t.Fatalf("Volume contract field %s contains %s", fields.Get(i).Name(), tok)
					}
				}
			}
		}
	}
	check((*quantramv1.VolumeEvent)(nil).ProtoReflect().Descriptor())
	check((*quantramv1.VolumeEmission)(nil).ProtoReflect().Descriptor())
	check((*quantramv1.VolumeSkip)(nil).ProtoReflect().Descriptor())
	check((*quantramv1.VolumeQuantity)(nil).ProtoReflect().Descriptor())
}

func TestC22C23ExistingContractsUnchanged(t *testing.T) {
	decision := (*quantramv1.DecisionEvent)(nil).ProtoReflect().Descriptor()
	assertField(t, decision, "event_id", 1)
	assertField(t, decision, "market_snapshot_id", 6)
	assertField(t, decision, "decision", 16)
	assertField(t, decision, "skip", 17)

	price := (*quantramv1.PriceEvent)(nil).ProtoReflect().Descriptor()
	assertField(t, price, "event_id", 1)
	assertField(t, price, "interval_start_unix_ms", 3)
	assertField(t, price, "market_snapshot_id", 4)
	assertField(t, price, "status", 8)
	assertField(t, price, "rk_success", 11)
	assertField(t, price, "emission", 12)
	assertField(t, price, "skip", 13)
	assertField(t, price, "cockpit", 14)

	if int32(quantramv1.PricingStatus_PRICING_STATUS_EMITTED) != 4 {
		t.Fatal("P-04 PricingStatus numeric values changed")
	}
	if int32(quantramv1.Side_SIDE_HOLD) != 3 {
		t.Fatal("P-03 Side numeric values changed")
	}
}

func TestC24C25ServiceExposure(t *testing.T) {
	svc := quantramv1.File_quantram_v1_quantram_proto.Services().ByName("ModelService")
	if svc == nil {
		t.Fatal("ModelService missing")
	}
	if svc.Methods().ByName("StreamDecisions") == nil || svc.Methods().ByName("StreamPriceEvents") == nil {
		t.Fatal("C24 existing ModelService methods missing")
	}
	vol := svc.Methods().ByName("StreamVolumeEvents")
	if vol == nil {
		t.Fatal("C25 StreamVolumeEvents missing")
	}
	if !vol.IsStreamingServer() || vol.IsStreamingClient() {
		t.Fatal("C25 StreamVolumeEvents must be server streaming")
	}
	if vol.Input().Name() != "StreamVolumeEventsRequest" || vol.Output().Name() != "VolumeEvent" {
		t.Fatalf("C25 signatures %s -> %s", vol.Input().Name(), vol.Output().Name())
	}
	if quantramv1.File_quantram_v1_quantram_proto.Services().ByName("VolumeService") != nil {
		t.Fatal("do not invent a Volume microservice")
	}
}

func TestStreamVolumeEventsUnwired(t *testing.T) {
	err := New(ingestion.NewPipeline(nil, nil, "TEST", []string{"SPY"}), nil).
		StreamVolumeEvents(&quantramv1.StreamVolumeEventsRequest{}, nil)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unwired want FailedPrecondition, got %v", err)
	}
}

func TestVolumeMapperInterpretationAndGV(t *testing.T) {
	ev := toProtoVolumeEvent(domain.VolumeEvent{
		Status:          domain.VolumeStatusAvailable,
		Reason:          "AVAILABLE",
		VN:              domain.VolumeQuantity{Value: 1.25, Status: domain.VolumeQtyAvailable},
		PredictedNextVN: domain.VolumeQuantity{Value: 1.25, Status: domain.VolumeQtyAvailable},
		RawColor:        domain.VolumeColorGreen,
		Indicator:       domain.VolumeColorAmber,
		Transition:      "PENDING_GREEN",
		Phase:           domain.VolumePhaseIncreasingAccelerating,
		Confidence:      domain.VolumeConfidenceHigh,
		DomainState:     domain.VolumeDomainCausalLocal,
	})
	em := ev.GetEmission()
	if em.GetRawColor() == em.GetIndicator() {
		t.Fatal("raw color must remain distinct from Indicator")
	}
	if em.GetRawColor() != quantramv1.VolumeIndicator_VOLUME_INDICATOR_GREEN {
		t.Fatal("raw color")
	}
	if em.GetIndicator() != quantramv1.VolumeIndicator_VOLUME_INDICATOR_AMBER {
		t.Fatal("Indicator")
	}
	if em.GetTransition() != quantramv1.VolumeTransition_VOLUME_TRANSITION_PENDING_GREEN {
		t.Fatal("transition")
	}
	if em.GetPredictedNextVN().GetValue() != em.GetVN().GetValue() {
		t.Fatal("G_V predicted_next_v_n must equal V_N")
	}
	if em.GetConfidence() != quantramv1.VolumeConfidence_VOLUME_CONFIDENCE_HIGH {
		t.Fatal("confidence")
	}
	if em.GetDomainState() != quantramv1.VolumeDomainState_VOLUME_DOMAIN_STATE_CAUSAL_LOCAL_VOLUME {
		t.Fatal("domain")
	}
	if ev.ProtoReflect().Descriptor().Fields().ByName("effective_time") != nil ||
		ev.ProtoReflect().Descriptor().Fields().ByName("effective_time_unix_ms") != nil {
		t.Fatal("do not invent Volume EffectiveTime")
	}
}

func TestVolumeDoesNotSerializeInternalState(t *testing.T) {
	fields := (*quantramv1.VolumeEvent)(nil).ProtoReflect().Descriptor().Fields()
	for _, name := range []protoreflect.Name{
		"pending_color", "pending_count", "raw_window", "vn_window", "generation",
	} {
		if fields.ByName(name) != nil {
			t.Fatalf("internal state leaked: %s", name)
		}
	}
}

func assertField(t *testing.T, desc protoreflect.MessageDescriptor, name protoreflect.Name, number protoreflect.FieldNumber) {
	t.Helper()
	fd := desc.Fields().ByName(name)
	if fd == nil {
		t.Fatalf("missing %s.%s", desc.Name(), name)
	}
	if fd.Number() != number {
		t.Fatalf("%s.%s number %d want %d", desc.Name(), name, fd.Number(), number)
	}
}
