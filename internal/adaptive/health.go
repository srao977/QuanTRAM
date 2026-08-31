package adaptive

func evaluateHealth(state *RuntimeState) string {
	values := []float64{
		state.StateVector.Level,
		state.StateVector.Velocity,
		state.StateVector.Acceleration,
		state.StateVector.Curvature,
		state.StateVector.Strength,
		state.StateVector.Persistence,
		state.StateVector.Uncertainty,
		state.StateVector.ReversalPropensity,
		state.HalfLife.ObservationHalfLife,
		state.HalfLife.ForwardHalfLife,
	}
	for _, v := range values {
		if !finite(v) {
			state.NonfiniteCount++
			return "INVALID"
		}
	}
	if state.DataGapCount > 0 {
		return "DEGRADED_DATA"
	}
	if state.ClippingCount > 0 || state.ParameterBoundHits > 0 {
		return "DEGRADED_NUMERICAL"
	}
	if state.StateVector.PerturbationMagnitude > 0.7 {
		return "PERTURBED"
	}
	return "HEALTHY"
}
