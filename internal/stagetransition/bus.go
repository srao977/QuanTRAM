package stagetransition

import (
	"sync"
	"sync/atomic"
)

// Publisher is a bounded in-process fan-out for Event values.
//
// Purpose:
//
//	Nonblocking Publish onto per-subscriber channels. Realtime callers must
//	not wait for disk, Snapshot, or subscriber processing.
//
// Buffer:
//
//	Each subscriber owns a channel of DefaultSubscriberBuffer (128) unless
//	Subscribe is given a positive override. Full channels drop that event
//	for that subscriber and increment Dropped.
//
// Zero subscribers:
//
//	Publish is a no-op besides the published counter. Safe and required.
//
// Shutdown:
//
//	Close unregisters and closes remaining subscriber channels. Further
//	Publish calls are ignored.
type Publisher struct {
	mu     sync.Mutex
	closed bool
	next   uint64
	subs   map[uint64]*subscription

	published atomic.Uint64
	delivered atomic.Uint64
	dropped   atomic.Uint64
}

type subscription struct {
	ch     chan Event
	closed atomic.Bool
}

type Stats struct {
	Published   uint64
	Delivered   uint64
	Dropped     uint64
	Subscribers int
}

func NewPublisher() *Publisher {
	return &Publisher{subs: make(map[uint64]*subscription)}
}

func (p *Publisher) Subscribe(buffer int) (uint64, <-chan Event) {
	if p == nil {
		ch := make(chan Event)
		close(ch)
		return 0, ch
	}
	if buffer <= 0 {
		buffer = DefaultSubscriberBuffer
	}
	ch := make(chan Event, buffer)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		close(ch)
		return 0, ch
	}
	id := p.next + 1
	p.next = id
	p.subs[id] = &subscription{ch: ch}
	return id, ch
}

func (p *Publisher) Unsubscribe(id uint64) {
	if p == nil || id == 0 {
		return
	}
	p.mu.Lock()
	sub, ok := p.subs[id]
	if ok {
		delete(p.subs, id)
	}
	p.mu.Unlock()
	if ok && sub.closed.CompareAndSwap(false, true) {
		close(sub.ch)
	}
}

func (p *Publisher) Publish(ev Event) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.published.Add(1)
	subs := make([]*subscription, 0, len(p.subs))
	for _, sub := range p.subs {
		subs = append(subs, sub)
	}
	p.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.ch <- ev:
			p.delivered.Add(1)
		default:
			p.dropped.Add(1)
		}
	}
}

func (p *Publisher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	subs := p.subs
	p.subs = make(map[uint64]*subscription)
	p.mu.Unlock()
	for _, sub := range subs {
		if sub.closed.CompareAndSwap(false, true) {
			close(sub.ch)
		}
	}
}

func (p *Publisher) Stats() Stats {
	if p == nil {
		return Stats{}
	}
	p.mu.Lock()
	n := len(p.subs)
	p.mu.Unlock()
	return Stats{
		Published:   p.published.Load(),
		Delivered:   p.delivered.Load(),
		Dropped:     p.dropped.Load(),
		Subscribers: n,
	}
}
