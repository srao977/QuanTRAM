package pricing

// This file refines projected motion into persistent cockpit state and color.

import (
	"math"

	"quantram/internal/domain"
)

// CockpitState retains motion, opposing-direction, and turn-candidate history
// across pricing emissions.
type CockpitState struct {
	PreviousMotionState string
	PreviousColor       string
	OpposingDirection   string
	OpposingCount       int
	CandidateDirection  string
	CandidateAge        int
}

type cockpitInterpreter struct {
	cfg Config
}

func newCockpit(cfg Config) cockpitInterpreter {
	return cockpitInterpreter{cfg: cfg}
}

// observe classifies motion, measures approach to zero velocity, and applies
// persistence and candidate hysteresis before choosing the cockpit color.
func (c cockpitInterpreter) observe(em domain.PriceEmission, state CockpitState) (domain.PriceCockpit, CockpitState) {
	values := []float64{em.P, em.P1, em.P2, em.ProjectedP, em.ProjectedP1, em.ProjectedP2}
	if !em.RKSuccess || !allFinite(values...) {
		reason := "RK_FAILURE"
		if em.RKSuccess {
			reason = "NONFINITE_TRAJECTORY"
		}
		return domain.PriceCockpit{
			Symbol:               em.Symbol,
			Timestamp:            em.Timestamp,
			Engine:               "P",
			RawPhase:             em.TrajectoryPhase,
			RefinedInternalState: "INVALID",
			P1ZeroProximity:      math.NaN(),
			DecelerationStrength: math.NaN(),
			PersistenceState:     "NONE",
			TurnCandidate:        "NONE",
			DomainState:          em.DomainState,
			ConfidenceState:      em.ConfidenceState,
			RawDirection:         "INVALID",
			CockpitColor:         "INVALID",
			ReasonCodes:          []string{reason},
		}, CockpitState{PreviousColor: "INVALID"}
	}

	eps := c.cfg.Epsilon
	p1 := em.P1
	projectedP1 := em.ProjectedP1
	rawDir := "NEAR_ZERO"
	if p1 > eps {
		rawDir = "UP"
	} else if p1 < -eps {
		rawDir = "DOWN"
	}
	motion := c.motionState(p1, em.P2, projectedP1)
	velocityScale := math.Max(math.Abs(p1), math.Abs(projectedP1))
	if velocityScale < eps {
		velocityScale = eps
	}
	zeroProx := math.Abs(projectedP1) / velocityScale
	opposingChange := 0.0
	if rawDir == "UP" {
		opposingChange = math.Max(0, p1-projectedP1)
	} else if rawDir == "DOWN" {
		opposingChange = math.Max(0, projectedP1-p1)
	}
	decScale := math.Abs(p1)
	if decScale < eps {
		decScale = eps
	}
	decStrength := opposingChange / decScale

	opposingDir := ""
	if p1 > eps && em.P2 < 0 && projectedP1 < p1 {
		opposingDir = "DOWN"
	} else if p1 < -eps && em.P2 > 0 && projectedP1 > p1 {
		opposingDir = "UP"
	}
	opposingCount := 0
	if opposingDir != "" && opposingDir == state.OpposingDirection {
		opposingCount = state.OpposingCount + 1
	} else if opposingDir != "" {
		opposingCount = 1
	}
	persistState := "NONE"
	if opposingDir != "" {
		persistState = opposingDir + "_DECELERATION"
	}

	crossing := ""
	if p1 > eps && projectedP1 <= 0 {
		crossing = "DOWN"
	} else if p1 < -eps && projectedP1 >= 0 {
		crossing = "UP"
	}
	nearZero := zeroProx <= c.cfg.ZeroProximityThreshold
	strongDec := decStrength >= c.cfg.DecelerationStrengthThreshold
	persistent := opposingCount >= c.cfg.PersistenceObservations

	candidate := ""
	reasons := []string{motion}
	if crossing != "" {
		candidate = crossing
		reasons = append(reasons, "PROJECTED_P1_"+crossing+"_CROSS")
	} else if opposingDir != "" && persistent && nearZero && strongDec {
		candidate = opposingDir
		reasons = append(reasons, "PERSISTENT_DECELERATION", "PROJECTED_P1_ZERO_APPROACH")
	}

	candidateAge := 0
	if candidate != "" {
		if state.CandidateDirection == candidate {
			candidateAge = state.CandidateAge + 1
		} else {
			candidateAge = 1
		}
	} else if state.CandidateDirection != "" && state.CandidateAge < c.cfg.CandidateHoldObservations {
		candidate = state.CandidateDirection
		candidateAge = state.CandidateAge + 1
		reasons = append(reasons, "CANDIDATE_HYSTERESIS")
	}

	directChange := candidate == "" && (rawDir == "UP" || rawDir == "DOWN") &&
		((state.PreviousColor == "GREEN" && rawDir == "DOWN") || (state.PreviousColor == "RED" && rawDir == "UP"))
	if directChange {
		reasons = append(reasons, "CURRENT_P1_DIRECTION_CROSS")
	}
	confCaution := em.ConfidenceState == "LOW" && c.cfg.LowConfidenceRequiresAmber
	domainCaution := em.DomainState == "OUT_OF_DOMAIN" && c.cfg.DomainExitRequiresAmber
	if confCaution {
		reasons = append(reasons, "LOW_CONFIDENCE")
	}
	if domainCaution {
		reasons = append(reasons, "DOMAIN_CAUTION")
	}

	internal := motion
	color := "GREEN"
	if rawDir == "DOWN" {
		color = "RED"
	}
	switch {
	case rawDir == "NEAR_ZERO":
		internal = "NEAR_STATIONARY"
		color = "AMBER"
		reasons = append(reasons, "NEAR_STATIONARY")
	case candidate != "":
		internal = "TURN_" + candidate + "_CANDIDATE"
		color = "AMBER"
	case directChange:
		internal = "DIRECTION_CHANGE_TRANSITION"
		color = "AMBER"
	case confCaution || domainCaution:
		internal = "UNCERTAIN"
		color = "AMBER"
	}

	out := domain.PriceCockpit{
		Symbol:               em.Symbol,
		Timestamp:            em.Timestamp,
		Engine:               "P",
		RawPhase:             em.TrajectoryPhase,
		RefinedInternalState: internal,
		P1ZeroProximity:      zeroProx,
		DecelerationStrength: decStrength,
		PersistenceState:     persistState,
		PersistenceCount:     opposingCount,
		TurnCandidate:        "NONE",
		CandidateAge:         candidateAge,
		DomainState:          em.DomainState,
		ConfidenceState:      em.ConfidenceState,
		RawDirection:         rawDir,
		CockpitColor:         color,
		ReasonCodes:          uniqueReasons(reasons),
	}
	if candidate != "" {
		out.TurnCandidate = candidate
	}
	next := CockpitState{
		PreviousMotionState: motion,
		PreviousColor:       color,
		OpposingDirection:   opposingDir,
		OpposingCount:       opposingCount,
		CandidateDirection:  candidate,
		CandidateAge:        candidateAge,
	}
	return out, next
}

func (c cockpitInterpreter) motionState(p1, p2, projectedP1 float64) string {
	eps := c.cfg.Epsilon
	if math.Abs(p1) <= eps {
		return "NEAR_STATIONARY"
	}
	if p1 > eps {
		if p2 > 0 && projectedP1 >= p1 {
			return "UP_ACCELERATING"
		}
		if p2 < 0 && projectedP1 < p1 {
			return "UP_DECELERATING"
		}
		return "UP_STABLE"
	}
	if p2 < 0 && projectedP1 <= p1 {
		return "DOWN_ACCELERATING"
	}
	if p2 > 0 && projectedP1 > p1 {
		return "DOWN_DECELERATING"
	}
	return "DOWN_STABLE"
}
