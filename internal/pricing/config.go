package pricing

// Config holds SADE PricingPipelineConfig and frozen policy/cockpit defaults.
// Do not recalibrate these values to hide gonum drift.
type Config struct {
	Entity                        string
	DerivativeWindow              int
	F4Window                      int
	Epsilon                       float64
	RidgeLambda                   float64
	RTOL                          float64
	DefaultSession                string
	DefaultSourceProvider         string
	EnableCockpit                 bool
	PolicyID                      string
	ConditionMedian               float64
	ConditionQ95                  float64
	EigenvalueMedian              float64
	EigenvalueQ95                 float64
	AmplificationMedian           float64
	AmplificationQ95              float64
	DirectReversalDebounce        bool
	CockpitPolicyID               string
	ZeroProximityThreshold        float64
	DecelerationStrengthThreshold float64
	PersistenceObservations       int
	CandidateHoldObservations     int
	LowConfidenceRequiresAmber    bool
	DomainExitRequiresAmber       bool
}

func DefaultConfig(entity string) Config {
	return Config{
		Entity:                        entity,
		DerivativeWindow:              15,
		F4Window:                      30,
		Epsilon:                       0.0035332071428566536,
		RidgeLambda:                   1.0,
		RTOL:                          1e-6,
		DefaultSession:                "UNKNOWN",
		DefaultSourceProvider:         "SDX_V1_1_STREAM",
		EnableCockpit:                 true,
		PolicyID:                      "P_EMISSION_V0_1",
		ConditionMedian:               7.835779770603297,
		ConditionQ95:                  13.040323846425492,
		EigenvalueMedian:              0.42217565243576405,
		EigenvalueQ95:                 0.6449378901835623,
		AmplificationMedian:           2.2423650649621742,
		AmplificationQ95:              2.6637448484678754,
		DirectReversalDebounce:        true,
		CockpitPolicyID:               "TRANSITION_EVIDENCE_P1",
		ZeroProximityThreshold:        0.9,
		DecelerationStrengthThreshold: 0.05,
		PersistenceObservations:       1,
		CandidateHoldObservations:     0,
		LowConfidenceRequiresAmber:    false,
		DomainExitRequiresAmber:       false,
	}
}

func (c Config) HistoryLimit() int {
	if c.DerivativeWindow > c.F4Window {
		return c.DerivativeWindow + 1
	}
	return c.F4Window + 1
}

// WarmupBars is the accepted-bar count before the first EXPM emission (15+30 on pricing_001).
func (c Config) WarmupBars() int {
	return c.DerivativeWindow + c.F4Window
}

func (c Config) Validate() error {
	if c.Entity == "" {
		return errf("CONFIG_INVALID entity must be non-empty")
	}
	if c.DerivativeWindow < 2 {
		return errf("CONFIG_INVALID derivative_window must be >= 2")
	}
	if c.F4Window < 3 {
		return errf("CONFIG_INVALID f4_window must be >= 3")
	}
	if c.Epsilon <= 0 {
		return errf("CONFIG_INVALID epsilon must be > 0")
	}
	if c.RidgeLambda < 0 {
		return errf("CONFIG_INVALID ridge_lambda must be >= 0")
	}
	if c.RTOL <= 0 {
		return errf("CONFIG_INVALID rtol must be > 0")
	}
	return nil
}
