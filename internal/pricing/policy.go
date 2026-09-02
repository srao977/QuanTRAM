package pricing

import (
	"math"

	"quantram/internal/domain"
)

type PolicyState struct {
	PreviousColor   string
	PendingReversal string
}

type EmissionPolicy struct {
	cfg Config
}

func NewEmissionPolicy(cfg Config) EmissionPolicy {
	return EmissionPolicy{cfg: cfg}
}

func direction(value, epsilon float64) string {
	if value > epsilon {
		return "UP"
	}
	if value < -epsilon {
		return "DOWN"
	}
	return "NEAR_ZERO"
}

func acceleration(value float64) string {
	if value > 0 {
		return "POSITIVE"
	}
	if value < 0 {
		return "NEGATIVE"
	}
	return "ZERO"
}

func phase(p1, p2, projectedP1, epsilon float64) string {
	if math.Abs(p1) <= epsilon {
		if projectedP1 > epsilon {
			return "TURNING_UP"
		}
		if projectedP1 < -epsilon {
			return "TURNING_DOWN"
		}
		return "NEAR_STATIONARY"
	}
	if p1 > epsilon && projectedP1 <= -epsilon {
		return "TURNING_DOWN"
	}
	if p1 < -epsilon && projectedP1 >= epsilon {
		return "TURNING_UP"
	}
	if p1 > 0 {
		if p2 > 0 {
			return "UP_ACCELERATING"
		}
		return "UP_DECELERATING"
	}
	if p2 > 0 {
		return "DOWN_DECELERATING"
	}
	return "DOWN_ACCELERATING"
}

func turningTendency(p1, p2, projectedP1, projectedP2, epsilon float64) string {
	if p1 > epsilon && projectedP1 <= -epsilon {
		return "TURNING_DOWN"
	}
	if p1 < -epsilon && projectedP1 >= epsilon {
		return "TURNING_UP"
	}
	if p1 > epsilon && p2 < 0 && projectedP1 < p1 {
		return "DETERIORATING_TOWARD_TURN"
	}
	if p1 < -epsilon && p2 > 0 && projectedP1 > p1 {
		return "RECOVERING_TOWARD_TURN"
	}
	if p2*projectedP2 < 0 {
		return "PROJECTED_P2_REVERSAL"
	}
	return "NONE"
}

func uniqueReasons(reasons []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, r := range reasons {
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

func (p EmissionPolicy) emit(n numericalRow, state PolicyState) (domain.PriceEmission, PolicyState) {
	finite := allFinite(n.P, n.P1, n.P2, n.ProjectedP, n.ProjectedP1, n.ProjectedP2)
	if !n.RKSuccess || !finite {
		reason := "RK_FAILURE"
		if n.RKSuccess {
			reason = "NONFINITE_TRAJECTORY"
		}
		em := p.build(n, "UNCERTAIN", "NONE", "INVALID", "INVALID", "INVALID", []string{reason}, "INVALID", "INVALID")
		return em, PolicyState{PreviousColor: "INVALID"}
	}
	ph := phase(n.P1, n.P2, n.ProjectedP1, p.cfg.Epsilon)
	tendency := turningTendency(n.P1, n.P2, n.ProjectedP1, n.ProjectedP2, p.cfg.Epsilon)
	reasons := []string{ph}
	if tendency != "NONE" {
		reasons = append(reasons, tendency)
	}
	domainState := "IN_DOMAIN"
	if n.DomainExit {
		domainState = "OUT_OF_DOMAIN"
	}
	confidence := "LOW"
	if domainState == "OUT_OF_DOMAIN" {
		reasons = append(reasons, "DOMAIN_EXIT", "LOW_CONFIDENCE")
	} else if n.ConditionNumber <= p.cfg.ConditionMedian && n.MaxRealEigenvalue <= p.cfg.EigenvalueMedian && n.PerturbationAmplification <= p.cfg.AmplificationMedian {
		confidence = "HIGH"
	} else if n.ConditionNumber <= p.cfg.ConditionQ95 && n.MaxRealEigenvalue <= p.cfg.EigenvalueQ95 && n.PerturbationAmplification <= p.cfg.AmplificationQ95 {
		confidence = "MEDIUM"
	} else {
		reasons = append(reasons, "LOW_CONFIDENCE")
	}

	stability := "STABLE"
	if n.MaxRealEigenvalue > 0 {
		stability = "LOCALLY_EXPANSIVE"
		reasons = append(reasons, "POSITIVE_MAX_REAL_EIGENVALUE")
	}

	raw := "AMBER"
	switch {
	case confidence == "LOW" || ph == "UP_DECELERATING" || ph == "DOWN_DECELERATING" || ph == "NEAR_STATIONARY" || ph == "UNCERTAIN":
		raw = "AMBER"
	case (ph == "UP_ACCELERATING" || ph == "TURNING_UP") && n.ProjectedP1 > p.cfg.Epsilon:
		raw = "GREEN"
	case (ph == "DOWN_ACCELERATING" || ph == "TURNING_DOWN") && n.ProjectedP1 < -p.cfg.Epsilon:
		raw = "RED"
	}

	color := raw
	pending := ""
	direct := (state.PreviousColor == "GREEN" && raw == "RED") || (state.PreviousColor == "RED" && raw == "GREEN")
	if p.cfg.DirectReversalDebounce && direct && state.PendingReversal != raw {
		color = "AMBER"
		pending = raw
		reasons = append(reasons, "DIRECT_REVERSAL_DEBOUNCE")
	} else if p.cfg.DirectReversalDebounce && state.PendingReversal == raw {
		pending = ""
	}

	em := p.build(n, ph, tendency, domainState, stability, confidence, reasons, raw, color)
	return em, PolicyState{PreviousColor: color, PendingReversal: pending}
}

func (p EmissionPolicy) build(n numericalRow, ph, tendency, domainState, stability, confidence string, reasons []string, raw, color string) domain.PriceEmission {
	return domain.PriceEmission{
		Symbol:                    n.Symbol,
		Timestamp:                 n.Timestamp,
		Engine:                    "P",
		P:                         n.P,
		P1:                        n.P1,
		P2:                        n.P2,
		ProjectedP:                n.ProjectedP,
		ProjectedP1:               n.ProjectedP1,
		ProjectedP2:               n.ProjectedP2,
		DeltaProjectedP:           n.ProjectedP - n.P,
		DeltaProjectedP1:          n.ProjectedP1 - n.P1,
		DeltaProjectedP2:          n.ProjectedP2 - n.P2,
		CurrentDirection:          direction(n.P1, p.cfg.Epsilon),
		CurrentAcceleration:       acceleration(n.P2),
		ProjectedDirection:        direction(n.ProjectedP1, p.cfg.Epsilon),
		ProjectedAcceleration:     acceleration(n.ProjectedP2),
		TrajectoryPhase:           ph,
		TurningTendency:           tendency,
		DomainState:               domainState,
		StabilityState:            stability,
		ConfidenceState:           confidence,
		RawColor:                  raw,
		Color:                     color,
		ReasonCodes:               uniqueReasons(reasons),
		RKSuccess:                 n.RKSuccess,
		ConditionNumber:           n.ConditionNumber,
		MaxRealEigenvalue:         n.MaxRealEigenvalue,
		PerturbationAmplification: n.PerturbationAmplification,
	}
}
