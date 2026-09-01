package ingestion

import (
	"log"
	"sync/atomic"
	"time"

	"quantram/internal/config"
	"quantram/internal/domain"
)

// FinalizedBarConsumer is the P-03 model path. Observe Subscribe/SubscribeFinalized stay lossy.
type FinalizedBarConsumer interface {
	SubscribeModelBars(buffer int) (uint64, <-chan domain.Bar)
	Unsubscribe(id uint64)
	ReadinessFor(symbol string) domain.Readiness
	Window(symbol string, limit int) []domain.Bar
	ModelPathStatus(symbol string) ModelPathStatus
	ResetModelPath(symbol string)
}

var _ FinalizedBarConsumer = (*Pipeline)(nil)

type ModelPathStatus struct {
	Discontinuous bool
	Reason        domain.SkipReason
	LastInterval  time.Time
	HasLast       bool
}

func (p *Pipeline) SubscribeModelBars(buffer int) (uint64, <-chan domain.Bar) {
	if buffer <= 0 {
		buffer = config.ConsumerQueue
	}
	id := atomic.AddUint64(&p.nextSub, 1)
	ch := make(chan domain.Bar, buffer)
	p.mu.Lock()
	p.modelSubs[id] = ch
	p.mu.Unlock()
	return id, ch
}

func (p *Pipeline) fanoutModel(bar domain.Bar) {
	if !bar.ModelEligible() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.modelSubs) == 0 {
		return
	}
	if reason, ok := p.modelDisc[bar.Symbol]; ok {
		log.Printf("model path skip symbol=%s reason=%s interval=%s", bar.Symbol, reason, bar.IntervalStart.UTC().Format(time.RFC3339))
		return
	}
	if last, ok := p.modelLast[bar.Symbol]; ok {
		if !bar.IntervalStart.After(last) {
			return
		}
		if bar.IntervalStart.Sub(last) != time.Minute {
			p.modelDisc[bar.Symbol] = domain.SkipInputGap
			log.Printf("model path INPUT_GAP symbol=%s last=%s got=%s", bar.Symbol, last.UTC().Format(time.RFC3339), bar.IntervalStart.UTC().Format(time.RFC3339))
			return
		}
	}
	for id, ch := range p.modelSubs {
		select {
		case ch <- bar:
		default:
			p.modelDisc[bar.Symbol] = domain.SkipQueueOverflow
			log.Printf("model path QUEUE_OVERFLOW subscriber=%d symbol=%s interval=%s (no silent drop)", id, bar.Symbol, bar.IntervalStart.UTC().Format(time.RFC3339))
			return
		}
	}
	p.modelLast[bar.Symbol] = bar.IntervalStart
}

func (p *Pipeline) ModelPathStatus(symbol string) ModelPathStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := ModelPathStatus{}
	if reason, ok := p.modelDisc[symbol]; ok {
		status.Discontinuous = true
		status.Reason = reason
	}
	if last, ok := p.modelLast[symbol]; ok {
		status.LastInterval = last
		status.HasLast = true
	}
	return status
}

func (p *Pipeline) ResetModelPath(symbol string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.modelDisc, symbol)
	delete(p.modelLast, symbol)
}

func (p *Pipeline) ReadinessFor(symbol string) domain.Readiness {
	ready := p.Readiness()
	status := p.ModelPathStatus(symbol)
	if status.Discontinuous {
		ready.Infer = false
		ready.Message = string(status.Reason)
	}
	return ready
}
