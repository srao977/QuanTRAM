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
	Symbol                               string
	Timestamp                            string
	Engine                               string
	P, P1, P2                            float64
	ProjectedP, ProjectedP1, ProjectedP2 float64
	DeltaProjectedP                      float64
	DeltaProjectedP1                     float64
	DeltaProjectedP2                     float64
	CurrentDirection                     string
	CurrentAcceleration                  string
	ProjectedDirection                   string
	ProjectedAcceleration                string
	TrajectoryPhase                      string
	TurningTendency                      string
	DomainState                          string
	StabilityState                       string
	ConfidenceState                      string
	RawColor                             string
	Color                                string
	ReasonCodes                          []string
	RKSuccess                            bool
	ConditionNumber                      float64
	MaxRealEigenvalue                    float64
	PerturbationAmplification            float64
}

// PriceCockpit is the optional SADE cockpit interpreter output.
type PriceCockpit struct {
	Symbol               string
	Timestamp            string
	Engine               string
	RawPhase             string
	RefinedInternalState string
	P1ZeroProximity      float64
	DecelerationStrength float64
	PersistenceState     string
	PersistenceCount     int
	TurnCandidate        string
	CandidateAge         int
	DomainState          string
	ConfidenceState      string
	RawDirection         string
	CockpitColor         string
	ReasonCodes          []string
}

type PricingSkip struct {
	Reason PricingSkipReason
	Detail string
}

// PriceEvent is one terminal pricing outcome per considered bar.
type PriceEvent struct {
	EventID          string
	Symbol           string
	IntervalStart    time.Time
	MarketSnapshotID string
	SourceTimestamp  string
	AcceptedSequence int
	Status           PricingStatus
	Emitted          bool
	DomainExit       bool
	RKSuccess        bool
	Latency          time.Duration
	Emission         *PriceEmission
	Cockpit          *PriceCockpit
	Skip             *PricingSkip
}

func (e PriceEvent) IsEmission() bool {
	return e.Emission != nil && e.Skip == nil
}
