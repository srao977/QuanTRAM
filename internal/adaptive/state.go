package adaptive

import "maps"

type StateVector struct {
	Level                 float64
	Velocity              float64
	Acceleration          float64
	Curvature             float64
	Strength              float64
	Persistence           float64
	PerturbationMagnitude float64
	Uncertainty           float64
	ReversalPropensity    float64
	DecayRelevance        float64
}

func DefaultStateVector() StateVector {
	return StateVector{
		Uncertainty:        0.15,
		ReversalPropensity: 0.1,
		DecayRelevance:     1.0,
	}
}

type HalfLifeState struct {
	ObservationHalfLife float64
	ForwardHalfLife     float64
}

type RuntimeState struct {
	EntityID                 string
	ModelTime                float64
	Sequence                 int
	AdaptiveReference        float64
	AdaptiveScale            float64
	VolumeReference          float64
	PrevLevel                float64
	PrevVelocity             float64
	LastEventTime            *float64
	LastObservation          *Observation
	ParameterState           map[string]float64
	ParameterUpdateMagnitude map[string]float64
	StateVector              StateVector
	HalfLife                 HalfLifeState
	ClippingCount            int
	NonfiniteCount           int
	ParameterBoundHits       int
	InnovationExtremeCount   int
	DataGapCount             int
}

func NewRuntimeState(entityID string, baselineHalfLife float64) RuntimeState {
	return RuntimeState{
		EntityID:                 entityID,
		AdaptiveScale:            1.0,
		VolumeReference:          1.0,
		ParameterState:           map[string]float64{"ref_alpha": 0.05},
		ParameterUpdateMagnitude: map[string]float64{},
		StateVector:              DefaultStateVector(),
		HalfLife:                 HalfLifeState{ObservationHalfLife: baselineHalfLife, ForwardHalfLife: baselineHalfLife},
	}
}

func (s RuntimeState) Clone() RuntimeState {
	out := s
	out.ParameterState = maps.Clone(s.ParameterState)
	if out.ParameterState == nil {
		out.ParameterState = map[string]float64{}
	}
	out.ParameterUpdateMagnitude = maps.Clone(s.ParameterUpdateMagnitude)
	if out.ParameterUpdateMagnitude == nil {
		out.ParameterUpdateMagnitude = map[string]float64{}
	}
	if s.LastEventTime != nil {
		v := *s.LastEventTime
		out.LastEventTime = &v
	}
	if s.LastObservation != nil {
		cloned := s.LastObservation.Clone()
		out.LastObservation = &cloned
	}
	return out
}
