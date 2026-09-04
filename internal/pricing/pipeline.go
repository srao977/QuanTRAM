package pricing

// This file owns the causal pricing history and transactional step pipeline.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"quantram/internal/domain"
)

type history struct {
	timestamps []string
	minutes    []float64
	opens      []float64
	highs      []float64
	lows       []float64
	closes     []float64
	volumes    []float64
	p1         []float64
	p2         []float64
	jp         []float64
	limit      int
}

func newHistory(limit int) history {
	return history{limit: limit}
}

// clone isolates all history slices for transactional evaluation.
func (h history) clone() history {
	cp := history{limit: h.limit}
	cp.timestamps = append([]string(nil), h.timestamps...)
	cp.minutes = append([]float64(nil), h.minutes...)
	cp.opens = append([]float64(nil), h.opens...)
	cp.highs = append([]float64(nil), h.highs...)
	cp.lows = append([]float64(nil), h.lows...)
	cp.closes = append([]float64(nil), h.closes...)
	cp.volumes = append([]float64(nil), h.volumes...)
	cp.p1 = append([]float64(nil), h.p1...)
	cp.p2 = append([]float64(nil), h.p2...)
	cp.jp = append([]float64(nil), h.jp...)
	return cp
}

// append adds a pending observation and evicts the oldest row at the configured
// limit; derivative slots begin non-finite until their causal windows are ready.
func (h *history) append(obs Observation) {
	h.timestamps = append(h.timestamps, obs.Timestamp)
	h.minutes = append(h.minutes, obs.Minutes)
	h.opens = append(h.opens, obs.Open)
	h.highs = append(h.highs, obs.High)
	h.lows = append(h.lows, obs.Low)
	h.closes = append(h.closes, obs.Close)
	h.volumes = append(h.volumes, obs.Volume)
	h.p1 = append(h.p1, math.NaN())
	h.p2 = append(h.p2, math.NaN())
	h.jp = append(h.jp, math.NaN())
	if len(h.closes) > h.limit {
		h.timestamps = h.timestamps[1:]
		h.minutes = h.minutes[1:]
		h.opens = h.opens[1:]
		h.highs = h.highs[1:]
		h.lows = h.lows[1:]
		h.closes = h.closes[1:]
		h.volumes = h.volumes[1:]
		h.p1 = h.p1[1:]
		h.p2 = h.p2[1:]
		h.jp = h.jp[1:]
	}
}

func (h history) lastIndex() int { return len(h.closes) - 1 }

// Engine is the collocated P-04 pricing engine. It does not read Decision.side.
type Engine struct {
	cfg           Config
	hist          history
	policy        PolicyState
	cockpitState  CockpitState
	engine        PriceEngine
	cockpit       *cockpitInterpreter
	lastSourceRow *int
	received      int
	entity        string
}

// NewEngine creates a validated pricing pipeline with empty causal history.
func NewEngine(entity string) (*Engine, error) {
	cfg := DefaultConfig(entity)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	e := &Engine{
		cfg:    cfg,
		hist:   newHistory(cfg.HistoryLimit()),
		engine: NewPriceEngine(cfg),
		entity: entity,
	}
	if cfg.EnableCockpit {
		c := newCockpit(cfg)
		e.cockpit = &c
	}
	return e, nil
}

// Received returns the number of bars committed to pricing history.
func (e *Engine) Received() int { return e.received }

// WarmupBars returns the accepted-bar count before the first possible emission.
func (e *Engine) WarmupBars() int { return e.cfg.WarmupBars() }

// StateHash returns a compact identity of committed history and policy state.
func (e *Engine) StateHash() string {
	last := 0.0
	if n := len(e.hist.closes); n > 0 {
		last = e.hist.closes[n-1]
	}
	payload := fmt.Sprintf("%d|%d|%g|%s|%s", e.received, len(e.hist.closes), last, e.policy.PreviousColor, e.cockpitState.PreviousColor)
	sum := sha256.Sum256([]byte(payload))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func (e *Engine) clone() *Engine {
	out := *e
	out.hist = e.hist.clone()
	if e.lastSourceRow != nil {
		v := *e.lastSourceRow
		out.lastSourceRow = &v
	}
	if e.cockpit != nil {
		c := *e.cockpit
		out.cockpit = &c
	}
	return &out
}

func (e *Engine) adopt(src *Engine) {
	e.hist = src.hist
	e.policy = src.policy
	e.cockpitState = src.cockpitState
	e.engine = src.engine
	e.cockpit = src.cockpit
	e.lastSourceRow = src.lastSourceRow
	e.received = src.received
}

// Step prepares one bar and commits every accepted warmup or emission result.
func (e *Engine) Step(bar domain.Bar) domain.PriceEvent {
	event, working, commit := e.PrepareStep(bar)
	if commit {
		e.Commit(working)
	}
	return event
}

// PrepareStep evaluates on a clone and leaves committed state unchanged until
// the caller passes the returned working engine to Commit.
func (e *Engine) PrepareStep(bar domain.Bar) (domain.PriceEvent, *Engine, bool) {
	working := e.clone()
	event, commit := working.apply(bar)
	if !commit {
		return event, nil, false
	}
	return event, working, true
}

// Commit atomically adopts a successfully prepared pricing state.
func (e *Engine) Commit(working *Engine) {
	if working != nil {
		e.adopt(working)
	}
}

func (e *Engine) apply(bar domain.Bar) (domain.PriceEvent, bool) {
	started := time.Now()
	obs := ObservationFromBar(bar, e.cfg)
	if obs.Entity != e.entity {
		return e.event(obs, started, domain.PricingStatusUnspecified, &domain.PricingSkip{
			Reason: domain.PricingSkipInvalidInput,
			Detail: "ENTITY_MISMATCH",
		}, nil, nil, numericalRow{}), false
	}
	next := e.hist.clone()
	nextPolicy := e.policy
	nextCockpit := e.cockpitState
	next.append(obs)
	received := e.received + 1
	globalIndex := received - 1
	globalActive := globalIndex - 1
	index := next.lastIndex()
	active := index - 1

	commit := func() {
		e.hist = next
		e.received = received
		e.lastSourceRow = &received
	}

	if active < 0 {
		commit()
		return e.warmup(obs, started, domain.PricingStatusWarmupDerivative, domain.PricingSkipWarmupDerivative, globalIndex), true
	}

	p1, p2, _ := causalQuadraticAtIndex(next.minutes, next.closes, active, e.cfg.DerivativeWindow)
	next.p1[active] = p1
	next.p2[active] = p2
	if !finite(p1) || !finite(p2) {
		commit()
		return e.warmup(obs, started, domain.PricingStatusWarmupDerivative, domain.PricingSkipWarmupDerivative, globalActive), true
	}

	if active > 0 && finite(next.p2[active-1]) {
		next.jp[active] = next.p2[active] - next.p2[active-1]
	}

	jpReady := globalActive >= e.cfg.F4Window
	if jpReady {
		start := active - e.cfg.F4Window + 1
		if start < 0 {
			jpReady = false
		} else {
			for i := start; i <= active; i++ {
				if !finite(next.jp[i]) {
					jpReady = false
					break
				}
			}
		}
	}
	if !jpReady {
		commit()
		return e.warmup(obs, started, domain.PricingStatusWarmupF4, domain.PricingSkipWarmupF4, globalActive), true
	}

	fit := fitF4AtIndex(next.closes, next.p1, next.p2, next.jp, active, e.cfg.F4Window, e.cfg.RidgeLambda)
	if fit == nil {
		commit()
		return e.warmup(obs, started, domain.PricingStatusF4Unavailable, domain.PricingSkipF4Unavailable, globalActive), true
	}

	// The newest bar closes the causal window; projection belongs to the prior
	// active row, so its timestamp and market values must drive the emission.
	activeObs := Observation{
		Entity:    obs.Entity,
		Timestamp: next.timestamps[active],
		Minutes:   next.minutes[active],
		Open:      next.opens[active],
		High:      next.highs[active],
		Low:       next.lows[active],
		Close:     next.closes[active],
		Volume:    next.volumes[active],
		Session:   obs.Session,
		Source:    obs.Source,
		Snapshot:  obs.Snapshot,
		Interval:  time.UnixMilli(int64(next.minutes[active] * 60_000)).UTC(),
	}

	cover := solveCover(fit, next.closes[active], next.p1[active], next.p2[active], false)
	num := buildNumericalRow(activeObs, active, globalActive, fit, cover, next.closes[active], next.p1[active], next.p2[active])

	em, pol, err := e.engine.observe(activeObs, num, nextPolicy)
	if err != nil {
		return e.event(obs, started, domain.PricingStatusUnspecified, &domain.PricingSkip{
			Reason: domain.PricingSkipEngineError,
			Detail: err.Error(),
		}, nil, nil, num), false
	}
	nextPolicy = pol
	var cockpit *domain.PriceCockpit
	if e.cockpit != nil {
		c, st := e.cockpit.observe(em, nextCockpit)
		nextCockpit = st
		cockpit = &c
	}

	e.policy = nextPolicy
	e.cockpitState = nextCockpit
	commit()

	status := domain.PricingStatusEmitted
	var skip *domain.PricingSkip
	if !num.RKSuccess {
		status = domain.PricingStatusProjectionFailure
		skip = &domain.PricingSkip{Reason: domain.PricingSkipProjectionFail}
	}
	ev := e.event(activeObs, started, status, skip, &em, cockpit, num)
	ev.Emitted = true
	ev.RKSuccess = num.RKSuccess
	ev.DomainExit = num.DomainExit
	return ev, true
}

func (e *Engine) warmup(obs Observation, started time.Time, status domain.PricingStatus, reason domain.PricingSkipReason, logical int) domain.PriceEvent {
	ev := e.event(obs, started, status, &domain.PricingSkip{Reason: reason}, nil, nil, numericalRow{ObservationIndex: logical + 1})
	ev.AcceptedSequence = logical + 1
	return ev
}

func (e *Engine) event(obs Observation, started time.Time, status domain.PricingStatus, skip *domain.PricingSkip, em *domain.PriceEmission, cockpit *domain.PriceCockpit, num numericalRow) domain.PriceEvent {
	seq := num.ObservationIndex
	if seq == 0 {
		seq = e.received
	}
	return domain.PriceEvent{
		Symbol:           obs.Entity,
		IntervalStart:    obs.Interval,
		MarketSnapshotID: obs.Snapshot,
		SourceTimestamp:  obs.Timestamp,
		AcceptedSequence: seq,
		Status:           status,
		Latency:          time.Since(started),
		Emission:         em,
		Cockpit:          cockpit,
		Skip:             skip,
		RKSuccess:        num.RKSuccess,
		DomainExit:       num.DomainExit,
	}
}
