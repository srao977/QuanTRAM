package adaptive

// This file defines the canonical direct and forward model outputs.

// FMOSample is one time-offset state in the forward model trajectory.
type FMOSample struct {
	Tau                float64 `bson:"tau"`
	Level              float64 `bson:"level"`
	Velocity           float64 `bson:"velocity"`
	Uncertainty        float64 `bson:"uncertainty"`
	Strength           float64 `bson:"strength"`
	Persistence        float64 `bson:"persistence"`
	ReversalPropensity float64 `bson:"reversal_propensity"`
}

// DMOOutput is the complete direct model output for one committed D01 step.
type DMOOutput struct {
	ModelTime                float64            `bson:"model_time"`
	EntityID                 string             `bson:"entity_id"`
	ModelVersion             string             `bson:"model_version"`
	StateLevel               float64            `bson:"state_level"`
	StateVelocity            float64            `bson:"state_velocity"`
	StateAcceleration        float64            `bson:"state_acceleration"`
	StateCurvature           float64            `bson:"state_curvature"`
	Strength                 float64            `bson:"strength"`
	Coherence                float64            `bson:"coherence"`
	Persistence              float64            `bson:"persistence"`
	PerturbationMagnitude    float64            `bson:"perturbation_magnitude"`
	PerturbationClass        string             `bson:"perturbation_class"`
	Uncertainty              float64            `bson:"uncertainty"`
	ReversalPropensity       float64            `bson:"reversal_propensity"`
	StateSupportRatio        float64            `bson:"state_support_ratio"`
	ObservationHalfLife      float64            `bson:"observation_half_life"`
	ForwardHalfLife          float64            `bson:"forward_half_life"`
	ParameterState           map[string]float64 `bson:"parameter_state"`
	ParameterUpdateMagnitude map[string]float64 `bson:"parameter_update_magnitude"`
	DataQuality              float64            `bson:"data_quality"`
	ModelHealth              string             `bson:"model_health"`
	DMOSchemaVersion         string             `bson:"dmo_schema_version"`
	FMOSchemaVersion         string             `bson:"fmo_schema_version"`
	ConfigHash               string             `bson:"config_hash"`
	StateHash                string             `bson:"state_hash"`
	TraceID                  string             `bson:"trace_id"`
}

// FMOOutput is the sampled forward trajectory paired with a DMOOutput.
type FMOOutput struct {
	ModelTime      float64     `bson:"model_time"`
	EntityID       string      `bson:"entity_id"`
	IntervalLength float64     `bson:"interval_length"`
	Samples        []FMOSample `bson:"samples"`
}
