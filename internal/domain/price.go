package domain

import "time"

// PricingStatus matches SADE PricingPipeline step status names for equivalence.
type PricingStatus string

const (
	PricingStatusUnspecified      PricingStatus = ""
	PricingStatusWarmupDerivative PricingStatus = "WARMUP_DERIVATIVE"
	PricingStatusWarmupF4         PricingStatus = "WARMUP_F4"
	PricingStatusF4Unavailable    PricingStatus = "F4_FIT_UNAVAILABLE"
	PricingStatusEmitted          PricingStatus = "EMITTED"
	// PricingStatusProjectionFailure is the Go symbol. The stored string remains
	// RK45_FAILURE so frozen SADE Pricing Unit Run 001 CSV status compares.
	// Canonical semantic ID is PRICE_STATUS_PROJECTION_FAILURE. Production solver is EXPM.
	PricingStatusProjectionFailure PricingStatus = "RK45_FAILURE"
)

type PricingSkipReason string

const (
	PricingSkipUnspecified      PricingSkipReason = ""
	PricingSkipWarmupDerivative PricingSkipReason = "WARMUP_DERIVATIVE"
	PricingSkipWarmupF4         PricingSkipReason = "WARMUP_F4"
	PricingSkipF4Unavailable    PricingSkipReason = "F4_FIT_UNAVAILABLE"
	PricingSkipProjectionFail   PricingSkipReason = "PROJECTION_FAILURE"
	PricingSkipUnstable         PricingSkipReason = "NUMERICALLY_UNSTABLE"
	PricingSkipInvalidInput     PricingSkipReason = "INVALID_INPUT"
	PricingSkipTimeout          PricingSkipReason = "TIMEOUT"
	PricingSkipEngineError      PricingSkipReason = "ENGINE_ERROR"
	PricingSkipEnginePanic      PricingSkipReason = "ENGINE_PANIC"
	PricingSkipTimeTerm         PricingSkipReason = "ANALYTIC_TIME_TERM_UNSUPPORTED"
)

// PriceEmission is the SADE PriceEngine policy output. It is not BUY/SELL/HOLD.
type PriceEmission struct {
	Symbol                    string   `bson:"symbol"`
	Timestamp                 string   `bson:"timestamp"`
	Engine                    string   `bson:"engine"`
	P                         float64  `bson:"p"`
	P1                        float64  `bson:"p1"`
	P2                        float64  `bson:"p2"`
	ProjectedP                float64  `bson:"projected_p"`
	ProjectedP1               float64  `bson:"projected_p1"`
	ProjectedP2               float64  `bson:"projected_p2"`
	DeltaProjectedP           float64  `bson:"delta_projected_p"`
	DeltaProjectedP1          float64  `bson:"delta_projected_p1"`
	DeltaProjectedP2          float64  `bson:"delta_projected_p2"`
	CurrentDirection          string   `bson:"current_direction"`
	CurrentAcceleration       string   `bson:"current_acceleration"`
	ProjectedDirection        string   `bson:"projected_direction"`
	ProjectedAcceleration     string   `bson:"projected_acceleration"`
	TrajectoryPhase           string   `bson:"trajectory_phase"`
	TurningTendency           string   `bson:"turning_tendency"`
	DomainState               string   `bson:"domain_state"`
	StabilityState            string   `bson:"stability_state"`
	ConfidenceState           string   `bson:"confidence_state"`
	RawColor                  string   `bson:"raw_color"`
	Color                     string   `bson:"color"`
	ReasonCodes               []string `bson:"reason_codes"`
	RKSuccess                 bool     `bson:"rk_success"`
	ConditionNumber           float64  `bson:"condition_number"`
	MaxRealEigenvalue         float64  `bson:"max_real_eigenvalue"`
	PerturbationAmplification float64  `bson:"perturbation_amplification"`
}

// PriceCockpit is the optional SADE cockpit interpreter output.
type PriceCockpit struct {
	Symbol               string   `bson:"symbol"`
	Timestamp            string   `bson:"timestamp"`
	Engine               string   `bson:"engine"`
	RawPhase             string   `bson:"raw_phase"`
	RefinedInternalState string   `bson:"refined_internal_state"`
	P1ZeroProximity      float64  `bson:"p1_zero_proximity"`
	DecelerationStrength float64  `bson:"deceleration_strength"`
	PersistenceState     string   `bson:"persistence_state"`
	PersistenceCount     int      `bson:"persistence_count"`
	TurnCandidate        string   `bson:"turn_candidate"`
	CandidateAge         int      `bson:"candidate_age"`
	DomainState          string   `bson:"domain_state"`
	ConfidenceState      string   `bson:"confidence_state"`
	RawDirection         string   `bson:"raw_direction"`
	CockpitColor         string   `bson:"cockpit_color"`
	ReasonCodes          []string `bson:"reason_codes"`
}

type PricingSkip struct {
	Reason PricingSkipReason `bson:"reason"`
	Detail string            `bson:"detail"`
}

// PriceEvent is one terminal pricing outcome per considered bar.
type PriceEvent struct {
	EventID          string         `bson:"event_id"`
	Symbol           string         `bson:"symbol"`
	IntervalStart    time.Time      `bson:"interval_start_unix_ms"`
	MarketSnapshotID string         `bson:"market_snapshot_id"`
	SourceTimestamp  string         `bson:"source_timestamp"`
	AcceptedSequence int            `bson:"accepted_sequence"`
	Status           PricingStatus  `bson:"status"`
	Emitted          bool           `bson:"emitted"`
	DomainExit       bool           `bson:"domain_exit"`
	RKSuccess        bool           `bson:"rk_success"`
	Latency          time.Duration  `bson:"latency_ns"`
	Emission         *PriceEmission `bson:"emission,omitempty"`
	Cockpit          *PriceCockpit  `bson:"cockpit,omitempty"`
	Skip             *PricingSkip   `bson:"skip,omitempty"`
}

func (e PriceEvent) IsEmission() bool {
	return e.Emission != nil && e.Skip == nil
}
