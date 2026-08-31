package adaptive

type FMOSample struct {
	Tau                float64
	Level              float64
	Velocity           float64
	Uncertainty        float64
	Strength           float64
	Persistence        float64
	ReversalPropensity float64
}

type DMOOutput struct {
	ModelTime                float64
	EntityID                 string
	ModelVersion             string
	StateLevel               float64
	StateVelocity            float64
	StateAcceleration        float64
	StateCurvature           float64
	Strength                 float64
	Coherence                float64
	Persistence              float64
	PerturbationMagnitude    float64
	PerturbationClass        string
	Uncertainty              float64
	ReversalPropensity       float64
	StateSupportRatio        float64
	ObservationHalfLife      float64
	ForwardHalfLife          float64
	ParameterState           map[string]float64
	ParameterUpdateMagnitude map[string]float64
	DataQuality              float64
	ModelHealth              string
	DMOSchemaVersion         string
	FMOSchemaVersion         string
	ConfigHash               string
	StateHash                string
	TraceID                  string
}

type FMOOutput struct {
	ModelTime      float64
	EntityID       string
	IntervalLength float64
	Samples        []FMOSample
}
