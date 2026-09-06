package modelhost

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"quantram/internal/config"
	"quantram/internal/domain"
)

func collectVolume(t *testing.T, events <-chan domain.VolumeEvent, n int, timeout time.Duration) []domain.VolumeEvent {
	t.Helper()
	out := make([]domain.VolumeEvent, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("volume channel closed after %d", len(out))
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("volume timed out after %d events, want %d", len(out), n)
		}
	}
	return out
}

func TestG01VolumeEnginePerWorker(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAA", "BBB"}, Options{})
	if host.workers["AAA"].volume == nil || host.workers["BBB"].volume == nil {
		t.Fatal("G01 each worker must own a Volume engine")
	}
	if host.workers["AAA"].volume == host.workers["BBB"].volume {
		t.Fatal("G01 engines must not be shared")
	}
	if host.workers["AAA"].volume.Entity() != "AAA" || host.workers["BBB"].volume.Entity() != "BBB" {
		t.Fatal("G02 entity ownership")
	}
}

func TestG03SameBarAndIndependentCommit(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Pricing: config.PricingExpm})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	bar := finalBar("AAPL", start, 100)
	src.push(bar)
	dec := collect(t, host.Events(), 1, time.Second)[0]
	vol := collectVolume(t, vols, 1, time.Second)[0]
	if vol.Lineage.MarketSnapshotID != bar.MarketSnapshotID {
		t.Fatalf("G05 snapshot %s vs %s", vol.Lineage.MarketSnapshotID, bar.MarketSnapshotID)
	}
	if !vol.Lineage.IntervalStart.Equal(bar.IntervalStart) {
		t.Fatalf("G06 interval %s vs %s", vol.Lineage.IntervalStart, bar.IntervalStart)
	}
	if dec.MarketSnapshotID != vol.Lineage.MarketSnapshotID || !dec.IntervalStart.Equal(vol.Lineage.IntervalStart) {
		t.Fatal("same-bar P-03/P-04V mismatch")
	}
	if host.WorkerVolumeCommitted("AAPL") != 1 {
		t.Fatal("G04 Volume must commit independently")
	}
	if vol.AcceptedSequence != 1 {
		t.Fatalf("G18 accepted_sequence want 1 got %d", vol.AcceptedSequence)
	}
	if vol.Status != domain.VolumeStatusMaturing {
		t.Fatalf("first bar should mature, got %s", vol.Status)
	}
}

func TestG07CausalOrder(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		src.push(finalBar("AAPL", start.Add(time.Duration(i)*time.Minute), 100+float64(i)))
	}
	got := collectVolume(t, vols, 3, 2*time.Second)
	for i, ev := range got {
		want := start.Add(time.Duration(i) * time.Minute)
		if !ev.Lineage.IntervalStart.Equal(want) || ev.AcceptedSequence != i+1 {
			t.Fatalf("G07 order i=%d got %s seq=%d", i, ev.Lineage.IntervalStart, ev.AcceptedSequence)
		}
	}
}

func TestG08PriceFailureDoesNotRollbackVolume(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Pricing: config.PricingExpm})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	host.FailNextPricing()
	src.push(finalBar("AAPL", start, 100))
	dec := collect(t, host.Events(), 1, time.Second)[0]
	vol := collectVolume(t, vols, 1, time.Second)[0]
	if host.WorkerVolumeCommitted("AAPL") != 1 {
		t.Fatal("G08 Volume must remain committed")
	}
	if host.WorkerHasAccepted("AAPL") {
		t.Fatal("G17 lastAccepted must not advance on A+P failure")
	}
	if !dec.IsSkip() || dec.Skip.Reason != domain.SkipEngineError {
		t.Fatalf("G08 want explicit Price/A+P skip, got %+v", dec.Skip)
	}
	if vol.Status == domain.VolumeStatusError {
		t.Fatal("G08 Volume should not be ENGINE_ERROR")
	}
	if host.WorkerCompleted("AAPL") != 0 {
		t.Fatal("G30 Adaptive must not commit")
	}
}

func TestG09AdaptiveFailureDoesNotRollbackVolume(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Pricing: config.PricingExpm})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	host.FailNextAdaptive()
	src.push(finalBar("AAPL", start, 101))
	dec := collect(t, host.Events(), 1, time.Second)[0]
	_ = collectVolume(t, vols, 1, time.Second)
	if host.WorkerVolumeCommitted("AAPL") != 1 {
		t.Fatal("G09 Volume must remain committed")
	}
	if host.WorkerHasAccepted("AAPL") {
		t.Fatal("G17 lastAccepted must stay A+P-only")
	}
	if !dec.IsSkip() || dec.Skip.Reason != domain.SkipEngineError {
		t.Fatalf("G09 want explicit Adaptive skip, got %+v", dec.Skip)
	}
}

func TestG10VolumeFailureDoesNotRollbackAP(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Pricing: config.PricingExpm})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.push(finalBar("AAPL", start, 100))
	_ = collect(t, host.Events(), 1, time.Second)
	_ = collectVolume(t, vols, 1, time.Second)
	host.FailNextVolume()
	src.push(finalBar("AAPL", start.Add(time.Minute), 101))
	dec := collect(t, host.Events(), 1, time.Second)[0]
	vol := collectVolume(t, vols, 1, time.Second)[0]
	if vol.Status != domain.VolumeStatusError {
		t.Fatalf("G13 Volume failure must be explicit, got %s", vol.Status)
	}
	if host.WorkerVolumeCommitted("AAPL") != 1 {
		t.Fatal("G10 failed Volume must not increment commit count")
	}
	if !host.WorkerHasAccepted("AAPL") {
		t.Fatal("G10 A+P must still commit")
	}
	if host.WorkerCompleted("AAPL") != 2 {
		t.Fatalf("G10 Adaptive completed=%d", host.WorkerCompleted("AAPL"))
	}
	if dec.IsSkip() && dec.Skip.Reason == domain.SkipEngineError && dec.Skip.Detail == "injected volume failure" {
		t.Fatal("Volume failure must not become Adaptive skip")
	}
	if !host.WorkerVolumeDiscontinuous("AAPL") {
		t.Fatal("G14 Volume hole must latch Volume")
	}
	src.push(finalBar("AAPL", start.Add(2*time.Minute), 102))
	hole := collectVolume(t, vols, 1, time.Second)[0]
	if hole.Reason != "VOLUME_DISCONTINUOUS" {
		t.Fatalf("G14 subsequent Volume must stay explicit, got %s", hole.Reason)
	}
	_ = collect(t, host.Events(), 1, time.Second)
	if host.WorkerCompleted("AAPL") != 3 {
		t.Fatal("G14 Volume latch must not block A+P")
	}
}

func TestG11AdaptivePanicDoesNotDenyVolume(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{PanicOn: "AAPL"})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.push(finalBar("AAPL", start, 100))
	dec := collect(t, host.Events(), 1, time.Second)[0]
	vol := collectVolume(t, vols, 1, time.Second)[0]
	if host.WorkerVolumeCommitted("AAPL") != 1 {
		t.Fatal("G11 Volume must receive/commit before Adaptive panic")
	}
	if vol.Status == domain.VolumeStatusError && vol.Reason == "ENGINE_PANIC" {
		t.Fatal("G11 Volume must not inherit Adaptive panic")
	}
	if !dec.IsSkip() || dec.Skip.Reason != domain.SkipEnginePanic {
		t.Fatalf("G11 Adaptive panic still isolated, got %+v", dec.Skip)
	}
}

func TestG12VolumePanicDoesNotDenyAP(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{PanicOnVolume: "AAPL", Pricing: config.PricingExpm})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	src.push(finalBar("AAPL", start, 100))
	dec := collect(t, host.Events(), 1, time.Second)[0]
	vol := collectVolume(t, vols, 1, time.Second)[0]
	if vol.Status != domain.VolumeStatusError || vol.Reason != "ENGINE_PANIC" {
		t.Fatalf("G12 Volume panic must be explicit, got %s %s", vol.Status, vol.Reason)
	}
	if !host.WorkerHasAccepted("AAPL") {
		t.Fatal("G12 A+P must still get the opportunity")
	}
	if dec.IsSkip() && dec.Skip.Reason == domain.SkipEnginePanic {
		t.Fatal("G12 Volume panic must not mark Adaptive ENGINE_PANIC")
	}
}

func TestG15CommonGatePreventsVolume(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	_, vols := host.SubscribeVolumeEvents(8)
	src.setInfer(false)
	src.push(finalBar("AAPL", time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC), 100))
	dec := collect(t, host.Events(), 1, time.Second)[0]
	if !dec.IsSkip() || dec.Skip.Reason != domain.SkipInferOff {
		t.Fatalf("want INFER_OFF, got %+v", dec.Skip)
	}
	select {
	case ev := <-vols:
		t.Fatalf("G15 common gate must not offer Volume, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
	if host.WorkerVolumeCommitted("AAPL") != 0 {
		t.Fatal("G15 Volume must not consume")
	}
}

func TestG16LastAcceptedDoesNotGateVolume(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{Pricing: config.PricingExpm})
	_, vols := host.SubscribeVolumeEvents(8)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	host.FailNextPricing()
	src.push(finalBar("AAPL", start, 100))
	_ = collect(t, host.Events(), 1, time.Second)
	_ = collectVolume(t, vols, 1, time.Second)
	if host.WorkerHasAccepted("AAPL") {
		t.Fatal("lastAccepted should be false")
	}
	src.push(finalBar("AAPL", start.Add(time.Minute), 101))
	_ = collect(t, host.Events(), 1, time.Second)
	vol2 := collectVolume(t, vols, 1, time.Second)[0]
	if vol2.AcceptedSequence != 2 {
		t.Fatalf("G16 Volume must continue without lastAccepted, seq=%d", vol2.AcceptedSequence)
	}
}

func TestG19ResetSymbolClearsVolume(t *testing.T) {
	src := newFake(8)
	host, _ := startHost(t, src, []string{"AAPL", "MSFT"}, Options{})
	_, vols := host.SubscribeVolumeEvents(16)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		src.push(finalBar("AAPL", start.Add(time.Duration(i)*time.Minute), 100+float64(i)))
		src.push(finalBar("MSFT", start.Add(time.Duration(i)*time.Minute), 200+float64(i)))
	}
	_ = collectVolume(t, vols, 6, 2*time.Second)
	if host.WorkerVolumeCommitted("AAPL") != 3 || host.WorkerVolumeCommitted("MSFT") != 3 {
		t.Fatal("pre-reset commits")
	}
	if err := host.ResetSymbol("AAPL"); err != nil {
		t.Fatal(err)
	}
	if host.WorkerVolumeCommitted("AAPL") != 0 {
		t.Fatal("G19 AAPL Volume seq must reset")
	}
	if host.WorkerVolumeSnapshot("AAPL").Feature.Raw != nil && len(host.WorkerVolumeSnapshot("AAPL").Feature.Raw) != 0 {
		t.Fatal("G19 VolumeState rings must be empty")
	}
	if host.WorkerVolumeCommitted("MSFT") != 3 {
		t.Fatal("G19 other symbol must be unaffected")
	}
	src.push(finalBar("AAPL", start.Add(10*time.Minute), 111))
	fresh := collectVolume(t, vols, 1, time.Second)
	var aapl domain.VolumeEvent
	for _, ev := range fresh {
		if ev.Lineage.Symbol == "AAPL" {
			aapl = ev
		}
	}
	if aapl.AcceptedSequence != 1 || aapl.Status != domain.VolumeStatusMaturing {
		t.Fatalf("G20 post-reset maturation restart got seq=%d status=%s", aapl.AcceptedSequence, aapl.Status)
	}
}

func TestG22MultiSymbolIsolation(t *testing.T) {
	src := newFake(16)
	host, _ := startHost(t, src, []string{"AAA", "BBB"}, Options{})
	_, vols := host.SubscribeVolumeEvents(16)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	a1 := finalBar("AAA", start, 10)
	a1.Volume = 10
	b1 := finalBar("BBB", start, 90)
	b1.Volume = 90
	a2 := finalBar("AAA", start.Add(time.Minute), 11)
	a2.Volume = 11
	b2 := finalBar("BBB", start.Add(time.Minute), 91)
	b2.Volume = 91
	src.push(a1)
	src.push(b1)
	src.push(a2)
	src.push(b2)
	got := collectVolume(t, vols, 4, 2*time.Second)
	snapA := host.WorkerVolumeSnapshot("AAA")
	snapB := host.WorkerVolumeSnapshot("BBB")
	if len(snapA.Feature.Raw) != 2 || len(snapB.Feature.Raw) != 2 {
		t.Fatalf("G22 ring lengths A=%d B=%d", len(snapA.Feature.Raw), len(snapB.Feature.Raw))
	}
	if snapA.Feature.Raw[0] != 10 || snapA.Feature.Raw[1] != 11 || snapB.Feature.Raw[0] != 90 || snapB.Feature.Raw[1] != 91 {
		t.Fatalf("G22 contamination A=%v B=%v", snapA.Feature.Raw, snapB.Feature.Raw)
	}
	if host.WorkerVolumeCommitted("AAA") != 2 || host.WorkerVolumeCommitted("BBB") != 2 {
		t.Fatal("G22 commit counts")
	}
	_ = got
}

func TestG36SlowVolumeSubscriberDoesNotBlock(t *testing.T) {
	src := newFake(32)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	id, ch := host.SubscribeVolumeEvents(1)
	defer host.UnsubscribeVolumeEvents(id)
	start := time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		src.push(finalBar("AAPL", start.Add(time.Duration(i)*time.Minute), 100+float64(i)))
		_ = collect(t, host.Events(), 1, time.Second)
	}
	if host.WorkerVolumeCommitted("AAPL") != 8 {
		t.Fatalf("G36 science must continue, committed=%d", host.WorkerVolumeCommitted("AAPL"))
	}
	_ = ch
}

func TestG23SingleSubscribeModelBars(t *testing.T) {
	src := newFake(4)
	host, _ := startHost(t, src, []string{"AAPL"}, Options{})
	if src.subscribeCount() != 1 {
		t.Fatalf("G23 want one SubscribeModelBars, got %d", src.subscribeCount())
	}
	_ = host
}

func TestTFormatQtyNeverLiesZero(t *testing.T) {
	if formatQty(domain.VolumeQuantity{Status: domain.VolumeQtyInsufficient}) == "0" {
		t.Fatal("T04 insufficient must not print 0")
	}
	if formatQty(domain.VolumeQuantity{Status: domain.VolumeQtyUndefined}) == "0" {
		t.Fatal("undefined must not print 0")
	}
	if formatQty(domain.VolumeQuantity{Value: 0, Status: domain.VolumeQtyAvailable}) != "0" {
		t.Fatal("available zero must print 0")
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	logVolumeEvent(domain.VolumeEvent{
		Status: domain.VolumeStatusMaturing,
		VN:     domain.VolumeQuantity{Status: domain.VolumeQtyInsufficient},
		Lineage: domain.VolumeLineage{
			Symbol:        "AAPL",
			IntervalStart: time.Date(2026, 9, 1, 13, 30, 0, 0, time.UTC),
		},
	})
	line := buf.String()
	if !strings.Contains(line, "volume maturing") || !strings.Contains(line, "INSUFFICIENT") {
		t.Fatalf("T04 maturing line %q", line)
	}
	if strings.Contains(line, "vn=0") {
		t.Fatalf("T04 must not print unavailable as zero: %q", line)
	}
	if volumeLogContainsTrading(line) {
		t.Fatal("T08 trading token")
	}
}

func TestTInvalidVsErrorLogs(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	logVolumeEvent(domain.VolumeEvent{Status: domain.VolumeStatusInvalid, Reason: "UNDEFINED_VN", Lineage: domain.VolumeLineage{Symbol: "AAPL"}})
	logVolumeEvent(domain.VolumeEvent{Status: domain.VolumeStatusError, Reason: "ENTITY_MISMATCH", Lineage: domain.VolumeLineage{Symbol: "AAPL"}})
	logVolumeEvent(domain.VolumeEvent{
		Status:    domain.VolumeStatusAvailable,
		Indicator: domain.VolumeColorAmber,
		RawColor:  domain.VolumeColorGreen,
		VN:        domain.VolumeQuantity{Value: 1.2, Status: domain.VolumeQtyAvailable},
		Lineage:   domain.VolumeLineage{Symbol: "AAPL", MarketSnapshotID: "snap-x"},
	})
	out := buf.String()
	if !strings.Contains(out, "volume invalid") || !strings.Contains(out, "volume error") {
		t.Fatalf("T05 distinct logs: %s", out)
	}
	if !strings.Contains(out, "indicator=AMBER") || !strings.Contains(out, "raw=GREEN") {
		t.Fatalf("T06/T07 indicator vs raw: %s", out)
	}
	if !strings.Contains(out, "snapshot=snap-x") {
		t.Fatal("T09 lineage")
	}
	if volumeLogContainsTrading(out) {
		t.Fatal("T08")
	}
}
