package volume

import "math"

// Feature mathematics for P-04V Phase B+C: V_N, interval mean, VOLUME_POINT, V1/V2.
//
// Purpose: apply frozen Volume Feature Science that does not require interpretation.
//
// Inputs: a Volume Observation and a State (normally a Clone candidate).
//
// Outputs: FeatureResult distinguishing available, insufficient (maturation),
// and undefined (window due but nonfinite/non-positive math). State rings
// advance by one aligned position.
//
// Parameters: RawWindow=15, IntervalMeanWindow=15, VOLUME_POINT. Not env knobs.
//
// Ownership: mutates the provided State only. Callers Clone before adopting.
//
// Lifecycle: AdvanceFeatures is a pure feature-prep helper, not Engine
// PrepareStep/Commit.
//
// Concurrency: caller serializes per-entity State.
//
// Failure: unavailable quantities are NaN with an explicit FeatureStatus.
// Zero raw with positive median yields V_N=0. Non-positive median yields
// undefined V_N, not an imputed value.
//
// Invariants: positional last-N; NaN keeps its index; rings stay aligned and
// bounded; predicted_next_V_N = V_N when V_N is available.
//
// Non-responsibilities: interpretation/color, proto, modelhost,
// Adaptive/Price science.

// FeatureStatus classifies one derived Volume feature quantity.
// It is an internal Phase B representation, not a proto enum and not
// Volume INVALID interpretation.
type FeatureStatus uint8

const (
	// FeatureInsufficient: not enough causal positions (maturation).
	FeatureInsufficient FeatureStatus = iota
	// FeatureAvailable: finite computed value.
	FeatureAvailable
	// FeatureUndefined: enough positions exist, but the quantity is
	// mathematically unavailable (non-positive/nonfinite prerequisite).
	FeatureUndefined
)

// Quantity is one feature value plus why it is or is not available.
type Quantity struct {
	Value  float64
	Status FeatureStatus
}

// Available reports a finite FeatureAvailable quantity.
func (q Quantity) Available() bool {
	return q.Status == FeatureAvailable && finite(q.Value)
}

// FeatureResult is the Phase B output of one observation's feature prep.
type FeatureResult struct {
	Observation  Observation
	VRaw         float64
	Median       Quantity
	VN           Quantity
	IntervalMean Quantity
	Predicted    Quantity
	V1           Quantity
	V2           Quantity
}

// AdvanceFeatures appends one positional observation and derives Phase B
// features on s. Use a Clone when committed isolation is required.
//
// Sequence: include current V_RAW in the raw window → median/V_N → append
// aligned (raw, vn-or-NaN, time) → interval mean of last 15 positional V_N →
// VOLUME_POINT = V_N → V1/V2 from last 3 positional VN and Times.
func (s *State) AdvanceFeatures(obs Observation) FeatureResult {
	out := FeatureResult{Observation: obs, VRaw: obs.VRaw}
	if s == nil {
		out.Median = unavailable(FeatureUndefined)
		out.VN = unavailable(FeatureUndefined)
		out.IntervalMean = unavailable(FeatureUndefined)
		out.Predicted = unavailable(FeatureUndefined)
		out.V1 = unavailable(FeatureUndefined)
		out.V2 = unavailable(FeatureUndefined)
		return out
	}

	rawAfter := append(append([]float64(nil), s.Feature.Raw...), obs.VRaw)
	out.Median, out.VN = normalizeFromRaw(rawAfter, obs.VRaw)

	vnStore := math.NaN()
	if out.VN.Available() {
		vnStore = out.VN.Value
	}
	s.AppendPositional(obs.VRaw, vnStore, obs.Lineage.IntervalStart)

	out.IntervalMean = intervalMeanVN(s.Feature.VN)
	out.Predicted = volumePoint(out.VN)
	deriv := derivativesFromState(s.Feature.VN, s.Feature.Times)
	out.V1 = deriv.V1
	out.V2 = deriv.V2
	return out
}

// normalizeFromRaw uses the last 15 positional raw values including current.
func normalizeFromRaw(rawAfter []float64, current float64) (median, vn Quantity) {
	if len(rawAfter) < RawWindow {
		return unavailable(FeatureInsufficient), unavailable(FeatureInsufficient)
	}
	window := rawAfter[len(rawAfter)-RawWindow:]
	med, ok := rollingMedian15(window)
	if !ok {
		return unavailable(FeatureUndefined), unavailable(FeatureUndefined)
	}
	median = Quantity{Value: med, Status: FeatureAvailable}
	if !finite(med) || med <= 0 {
		return median, unavailable(FeatureUndefined)
	}
	value := current / med
	if !finite(value) {
		return median, unavailable(FeatureUndefined)
	}
	return median, Quantity{Value: value, Status: FeatureAvailable}
}

// intervalMeanVN is the arithmetic mean of the last 15 positional V_N.
// Any nonfinite member makes the mean undefined; it does not skip backward.
func intervalMeanVN(vn []float64) Quantity {
	if len(vn) < IntervalMeanWindow {
		return unavailable(FeatureInsufficient)
	}
	window := vn[len(vn)-IntervalMeanWindow:]
	sum := 0.0
	for _, v := range window {
		if !finite(v) {
			return unavailable(FeatureUndefined)
		}
		sum += v
	}
	mean := sum / float64(IntervalMeanWindow)
	if !finite(mean) {
		return unavailable(FeatureUndefined)
	}
	return Quantity{Value: mean, Status: FeatureAvailable}
}

// volumePoint is frozen VOLUME_POINT: predicted_next_V_N = V_N.
func volumePoint(vn Quantity) Quantity {
	if !vn.Available() {
		return unavailable(vn.Status)
	}
	return Quantity{Value: vn.Value, Status: FeatureAvailable}
}

func unavailable(status FeatureStatus) Quantity {
	if status == FeatureAvailable {
		status = FeatureUndefined
	}
	return Quantity{Value: math.NaN(), Status: status}
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
