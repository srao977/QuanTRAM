package pricing

// This file binds numerical rows to the pricing emission policy.

import "quantram/internal/domain"

// PriceEngine validates row alignment before applying emission policy state.
type PriceEngine struct {
	policy EmissionPolicy
}

// NewPriceEngine creates a policy-backed pricing engine.
func NewPriceEngine(cfg Config) PriceEngine {
	return PriceEngine{policy: NewEmissionPolicy(cfg)}
}

func (e PriceEngine) observe(obs Observation, n numericalRow, state PolicyState) (domain.PriceEmission, PolicyState, error) {
	if obs.Entity != n.Symbol {
		return domain.PriceEmission{}, state, errf("observation symbol and trajectory symbol differ observation=%s trajectory=%s", obs.Entity, n.Symbol)
	}
	if n.Timestamp != obs.Timestamp {
		return domain.PriceEmission{}, state, errf("observation and trajectory timestamps differ")
	}
	em, next := e.policy.emit(n, state)
	return em, next, nil
}
