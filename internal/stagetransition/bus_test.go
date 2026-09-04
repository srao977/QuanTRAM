package stagetransition

import (
	"sync"
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestZeroSubscribersSafe(t *testing.T) {
	p := NewPublisher()
	p.Publish(Event{TransitionID: "x", Current: StageState{Code: "A"}})
	st := p.Stats()
	if st.Published != 1 || st.Delivered != 0 || st.Dropped != 0 || st.Subscribers != 0 {
		t.Fatalf("%+v", st)
	}
	var nilPub *Publisher
	nilPub.Publish(Event{})
	_, ch := nilPub.Subscribe(1)
	if _, ok := <-ch; ok {
		t.Fatal("nil subscribe must be closed")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	p := NewPublisher()
	_, a := p.Subscribe(4)
	_, b := p.Subscribe(4)
	p.Publish(Event{TransitionID: "t1", Current: StageState{Code: "B"}})
	if (<-a).TransitionID != "t1" || (<-b).TransitionID != "t1" {
		t.Fatal("fanout")
	}
}

func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	p := NewPublisher()
	_, slow := p.Subscribe(1)
	p.Publish(Event{TransitionID: "1", Current: StageState{Code: "A"}})
	done := make(chan struct{})
	go func() {
		p.Publish(Event{TransitionID: "2", Current: StageState{Code: "B"}})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish blocked on full subscriber")
	}
	if p.Stats().Dropped != 1 {
		t.Fatalf("dropped %+v", p.Stats())
	}
	<-slow
}

func TestFullSubscriberDoesNotBlockAndIsAccounted(t *testing.T) {
	p := NewPublisher()
	id, _ := p.Subscribe(1)
	p.Publish(Event{Current: StageState{Code: "A"}})
	p.Publish(Event{Current: StageState{Code: "B"}})
	st := p.Stats()
	if st.Published != 2 || st.Dropped != 1 || st.Delivered != 1 {
		t.Fatalf("%+v", st)
	}
	p.Unsubscribe(id)
}

func TestUnsubscribeAndCloseSafe(t *testing.T) {
	p := NewPublisher()
	id, ch := p.Subscribe(2)
	p.Unsubscribe(id)
	if _, ok := <-ch; ok {
		t.Fatal("unsub must close")
	}
	p.Unsubscribe(id)
	p.Close()
	p.Close()
	p.Publish(Event{Current: StageState{Code: "Z"}})
	if p.Stats().Published != 0 {
		t.Fatal("publish after close")
	}
	_, closed := p.Subscribe(1)
	if _, ok := <-closed; ok {
		t.Fatal("subscribe after close")
	}
}

func TestConcurrentPublishBounded(t *testing.T) {
	p := NewPublisher()
	_, ch := p.Subscribe(8)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Publish(Event{Current: StageState{Code: "X"}})
		}()
	}
	wg.Wait()
	st := p.Stats()
	if st.Published != 32 {
		t.Fatalf("published %d", st.Published)
	}
	if st.Delivered+st.Dropped != 32 {
		t.Fatalf("delivered+dropped %+v", st)
	}
	if cap(ch) != DefaultSubscriberBuffer && cap(ch) != 8 {
		t.Fatalf("cap %d", cap(ch))
	}
	p.Close()
}

func BenchmarkPublishZeroSubscribers(b *testing.B) {
	p := NewPublisher()
	ev := Event{Current: StageState{Code: "HOLD"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.Publish(ev)
	}
}

func BenchmarkHubUnchangedDecision(b *testing.B) {
	h := NewHub()
	start := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	ev := decision("AAPL", domain.SideHold, start, "a:1")
	bar := causalBar("AAPL", start, 100)
	h.OnDecision(ev, bar)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.OnDecision(ev, bar)
	}
}
