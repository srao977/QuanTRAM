package modelhost

import (
	"fmt"
	"log"
	"strings"

	"quantram/internal/domain"
	"quantram/internal/volume"
)

// P-04V host join (Phase G).
//
// Purpose: offer the same published Bar to the per-worker Volume Engine after
// common Host gates, commit Volume independently of Adaptive+Price, and publish
// VolumeEvents on the existing non-blocking subscriber pattern.
//
// Inputs: the domain.Bar already delivered to worker.handle after infer,
// eligibility, continuity, proven-missing, and pre-prepare timeout gates.
//
// Outputs: domain.VolumeEvent published to volume subscribers and last-event
// catch-up. Terminal lines via the standard library logger (observational).
//
// Parameters: none. Volume science uses FrozenConfig only. No Volume env mode.
//
// Ownership: each worker owns one volume.Engine. Mutable VolumeState is not
// shared across symbols. The keyed worker serializes calls.
//
// Lifecycle: constructed in Host.New; ResetSymbol rebuilds a cold engine;
// no automatic session/date/source reset.
//
// Concurrency: called only from the symbol worker. Publication is non-blocking
// (drop + log when a subscriber buffer is full), matching Price.
//
// Failure: ENGINE_ERROR and Volume panic latch Volume only (volDisc). Adaptive
// and Price retain their opportunity. A+P failure cannot roll back a Volume
// commit that already happened. Common-gate denial is not a Volume hole.
//
// Invariants: same MarketSnapshotID and IntervalStart as the initiating Bar;
// accepted_sequence is the worker Volume commit count, not lastAccepted;
// unavailable quantities are never printed as zero.
//
// Non-responsibilities: Volume mathematics, proto, StageTransition, Snapshot,
// P/V fusion, trading verbs, a second SubscribeModelBars, a Volume mailbox.

func (h *Host) processVolume(w *worker, bar domain.Bar) {
	if w == nil || w.volume == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			w.markVolDisc()
			ev := volumeErrorEvent(bar, "ENGINE_PANIC")
			h.finishVolumeEvent(w, &ev, false)
			h.emitVolume(ev)
			log.Printf("volume panic isolated symbol=%s err=%v", w.symbol, rec)
		}
	}()

	if w.volDisc.Load() {
		ev := volumeErrorEvent(bar, "VOLUME_DISCONTINUOUS")
		h.finishVolumeEvent(w, &ev, false)
		h.emitVolume(ev)
		return
	}
	if h.failVolume.CompareAndSwap(true, false) {
		w.markVolDisc()
		ev := volumeErrorEvent(bar, "INJECTED_VOLUME_FAILURE")
		h.finishVolumeEvent(w, &ev, false)
		h.emitVolume(ev)
		return
	}
	if h.opts.PanicOnVolume == w.symbol {
		panic("injected volume panic")
	}

	ev, working, ok := w.volume.PrepareStep(bar)
	if !ok {
		w.markVolDisc()
		h.finishVolumeEvent(w, &ev, false)
		h.emitVolume(ev)
		return
	}
	w.volume.Commit(working)
	h.finishVolumeEvent(w, &ev, true)
	h.emitVolume(ev)
}

func (h *Host) finishVolumeEvent(w *worker, ev *domain.VolumeEvent, committed bool) {
	if committed {
		w.volSeq++
		ev.AcceptedSequence = w.volSeq
	}
	if ev.EventID == "" {
		ev.EventID = fmt.Sprintf("%s:volume:%d", w.symbol, h.seq.Add(1))
	}
}

func volumeErrorEvent(bar domain.Bar, reason string) domain.VolumeEvent {
	return domain.VolumeEvent{
		Lineage: volume.ObservationFromBar(bar).Lineage,
		Status:  domain.VolumeStatusError,
		Reason:  reason,
		VRaw:    domain.VolumeQuantity{Value: float64(bar.Volume), Status: domain.VolumeQtyAvailable},
	}
}

func (w *worker) markVolDisc() {
	w.volDisc.Store(true)
}

func logVolumeEvent(ev domain.VolumeEvent) {
	switch ev.Status {
	case domain.VolumeStatusAvailable:
		log.Printf("volume emit symbol=%s status=%s indicator=%s raw=%s vn=%s v1=%s v2=%s mean15=%s next_vn=%s transition=%s phase=%s confidence=%s domain=%s snapshot=%s interval=%s",
			ev.Lineage.Symbol, ev.Status, displayOrNA(ev.Indicator),
			displayOrNA(ev.RawColor), formatQty(ev.VN), formatQty(ev.V1), formatQty(ev.V2),
			formatQty(ev.IntervalMeanVN), formatQty(ev.PredictedNextVN),
			displayOrNA(ev.Transition), displayOrNA(ev.Phase), displayOrNA(ev.Confidence),
			displayOrNA(ev.DomainState), ev.Lineage.MarketSnapshotID,
			ev.Lineage.IntervalStart.UTC().Format("2006-01-02T15:04:05Z"))
	case domain.VolumeStatusMaturing:
		log.Printf("volume maturing symbol=%s status=%s vn=%s v1=%s mean15=%s indicator=UNAVAILABLE snapshot=%s interval=%s",
			ev.Lineage.Symbol, ev.Status, formatQty(ev.VN), formatQty(ev.V1),
			formatQty(ev.IntervalMeanVN), ev.Lineage.MarketSnapshotID,
			ev.Lineage.IntervalStart.UTC().Format("2006-01-02T15:04:05Z"))
	case domain.VolumeStatusInvalid:
		log.Printf("volume invalid symbol=%s status=%s reason=%s vn=%s snapshot=%s interval=%s",
			ev.Lineage.Symbol, ev.Status, ev.Reason, formatQty(ev.VN),
			ev.Lineage.MarketSnapshotID, ev.Lineage.IntervalStart.UTC().Format("2006-01-02T15:04:05Z"))
	default:
		log.Printf("volume error symbol=%s status=%s reason=%s snapshot=%s interval=%s",
			ev.Lineage.Symbol, ev.Status, ev.Reason, ev.Lineage.MarketSnapshotID,
			ev.Lineage.IntervalStart.UTC().Format("2006-01-02T15:04:05Z"))
	}
}

func formatQty(q domain.VolumeQuantity) string {
	switch q.Status {
	case domain.VolumeQtyAvailable:
		return fmt.Sprintf("%g", q.Value)
	case domain.VolumeQtyUndefined:
		return "UNDEFINED"
	case domain.VolumeQtyInsufficient:
		return "INSUFFICIENT"
	default:
		return "UNAVAILABLE"
	}
}

func displayOrNA(value string) string {
	if strings.TrimSpace(value) == "" {
		return "UNAVAILABLE"
	}
	return value
}

func volumeLogContainsTrading(line string) bool {
	upper := strings.ToUpper(line)
	for _, tok := range []string{"BUY", "SELL", "HOLD"} {
		if strings.Contains(upper, tok) {
			return true
		}
	}
	return false
}
