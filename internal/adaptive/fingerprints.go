package adaptive

// SADE scientific_baseline.py — frozen identity for the proved adaptive path.
const (
	// BaselineRuleFingerprint identifies the frozen decision rules.
	BaselineRuleFingerprint = "c4c5bbf36ab97b3e7fc4628dfe11708947f996bcd79901a9d19b6a0f2049e9e2"
	// BaselineImplementationFingerprint identifies the frozen reference implementation.
	BaselineImplementationFingerprint = "e8b736dfba03b454633831585222d5270c18b7f8eae510b34ee19dc1f5c58410"
	// SchemaVersion identifies adaptive decision events.
	SchemaVersion = "quantram.adaptive.v1"
	// ModelVersionLabel identifies the D01 model version carried by events.
	ModelVersionLabel = "0.2"
	// ContextLength is the number of prior evaluations used by the decision rule.
	ContextLength = 15
	// ActionableAfter is the first accepted observation that can emit a decision.
	ActionableAfter = ContextLength + 1
)
