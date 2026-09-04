package stagetransition

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"quantram/internal/domain"
)

const DefaultLogPath = "./stage_transitions.txt"

const flushInterval = time.Second

// Diagnostic is a StageTransitionSubscriber that writes human-readable
// transition blocks to a text file.
//
// Purpose:
//
//	Live evidence of published Event values. Not Snapshot storage, not
//	replay authority, not an application input.
//
// Inputs:
//
//	Event values from Hub.Subscribe. Never reads the TXT file back.
//
// Outputs:
//
//	Buffered writes to path (default ./stage_transitions.txt, relative to
//	the server working directory). The normal start script runs from the
//	repository root.
//
// Ownership:
//
//	Subscriber goroutine owns all filesystem I/O. Publish does not format
//	or write.
//
// Lifecycle:
//
//	NewDiagnostic truncates the file and writes a run header. Close stops
//	accepting, drains the subscription, flushes, writes a footer, and
//	closes the file.
//
// Failure:
//
//	Open/write errors are counted and logged. They must not stop the
//	realtime path. A full subscriber queue drops that delivery only.
type Diagnostic struct {
	hub  *Hub
	id   uint64
	ch   <-chan Event
	path string

	written  atomic.Uint64
	dropped  atomic.Uint64
	writeErr atomic.Uint64

	stop   chan struct{}
	done   chan struct{}
	closed atomic.Bool
}

func NewDiagnostic(hub *Hub, path string) (*Diagnostic, error) {
	if hub == nil {
		return nil, fmt.Errorf("stage transition hub is nil")
	}
	if path == "" {
		path = DefaultLogPath
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open stage transition log %q: %w", path, err)
	}
	id, ch := hub.Subscribe(DefaultSubscriberBuffer)
	d := &Diagnostic{
		hub:  hub,
		id:   id,
		ch:   ch,
		path: path,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go d.loop(file)
	return d, nil
}

func (d *Diagnostic) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *Diagnostic) Written() uint64 {
	if d == nil {
		return 0
	}
	return d.written.Load()
}

func (d *Diagnostic) Dropped() uint64 {
	if d == nil {
		return 0
	}
	return d.dropped.Load()
}

func (d *Diagnostic) WriteErrors() uint64 {
	if d == nil {
		return 0
	}
	return d.writeErr.Load()
}

func (d *Diagnostic) Close() {
	if d == nil || !d.closed.CompareAndSwap(false, true) {
		return
	}
	close(d.stop)
	d.hub.Unsubscribe(d.id)
	<-d.done
}

func (d *Diagnostic) loop(file *os.File) {
	defer close(d.done)
	defer file.Close()

	w := bufio.NewWriterSize(file, 16*1024)
	if err := writeHeader(w, time.Now()); err != nil {
		d.writeErr.Add(1)
		log.Printf("stage transition diagnostic header: %v", err)
	}
	_ = w.Flush()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var stopSeen bool
	for {
		select {
		case ev, ok := <-d.ch:
			if !ok {
				_ = writeFooter(w, time.Now(), d.written.Load(), d.dropped.Load(), d.writeErr.Load())
				if err := w.Flush(); err != nil {
					d.writeErr.Add(1)
					log.Printf("stage transition diagnostic flush: %v", err)
				}
				return
			}
			if err := writeEvent(w, ev); err != nil {
				d.writeErr.Add(1)
				log.Printf("stage transition diagnostic write: %v", err)
				continue
			}
			d.written.Add(1)
		case <-ticker.C:
			if err := w.Flush(); err != nil {
				d.writeErr.Add(1)
				log.Printf("stage transition diagnostic flush: %v", err)
			}
		case <-d.stop:
			if stopSeen {
				continue
			}
			stopSeen = true
			d.dropped.Store(d.hub.Stats().Dropped)
		}
	}
}

func writeHeader(w io.Writer, started time.Time) error {
	_, err := fmt.Fprintf(w, `%s
QUANTRAM STAGE TRANSITION DIAGNOSTIC
%s

Process Started:    %s
Contract Version:   %s
Purpose:            Human-readable StageTransition diagnostic output

%s

`, banner, banner, formatTime(started), ContractVersion, banner)
	return err
}

func writeFooter(w io.Writer, stopped time.Time, written, dropped, writeErr uint64) error {
	_, err := fmt.Fprintf(w, `%s
STAGE TRANSITION DIAGNOSTIC COMPLETE
%s

Process Stopped:       %s
Transitions Written:  %d
Transitions Dropped:  %d
Write Errors:         %d

%s
`, banner, banner, formatTime(stopped), written, dropped, writeErr, banner)
	return err
}

func writeEvent(w io.Writer, ev Event) error {
	var b strings.Builder
	b.WriteString(line)
	b.WriteString("STAGE TRANSITION\n")
	b.WriteString(line)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Transition ID:      %s\n", na(ev.TransitionID))
	fmt.Fprintf(&b, "Sequence:           %s\n", formatSeq(ev.Sequence))
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Stage ID:           %s\n", na(string(ev.StageID)))
	fmt.Fprintf(&b, "Stage Name:         %s\n", na(ev.StageName))
	fmt.Fprintf(&b, "Entity:             %s\n", na(ev.EntityID))
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Effective Time:     %s\n", formatTime(ev.EffectiveEventTime))
	fmt.Fprintf(&b, "Published Time:     %s\n", formatTime(ev.PublishedTime))
	b.WriteByte('\n')
	b.WriteString("Previous State:\n")
	b.WriteString(formatStateLines("    ", ev.Previous))
	b.WriteByte('\n')
	b.WriteString("Current State:\n")
	b.WriteString(formatStateLines("    ", ev.Current))
	b.WriteByte('\n')
	b.WriteString("Initiating Bar:\n")
	writeInitiatingBar(&b, ev.InitiatingBar)
	b.WriteByte('\n')
	b.WriteString("Transition Facts:\n")
	writeFacts(&b, ev)
	b.WriteString("Correlation:\n")
	fmt.Fprintf(&b, "    Market Snapshot ID: %s\n", na(ev.MarketSnapshotID))
	fmt.Fprintf(&b, "    Accepted Sequence:  %s\n", formatAccepted(ev.AcceptedSequence, ev.SourceEventID != ""))
	fmt.Fprintf(&b, "    Source Event ID:    %s\n", na(ev.SourceEventID))
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Reason:\n    %s\n", na(ev.ReasonCode))
	b.WriteByte('\n')
	b.WriteString(line)
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err
}

func writeInitiatingBar(b *strings.Builder, bar *domain.Bar) {
	if bar == nil {
		b.WriteString("    N/A\n")
		return
	}
	fmt.Fprintf(b, "    Symbol:              %s\n", na(bar.Symbol))
	fmt.Fprintf(b, "    Market Snapshot ID:  %s\n", na(bar.MarketSnapshotID))
	fmt.Fprintf(b, "    Interval Start:      %s\n", formatTime(bar.IntervalStart))
	fmt.Fprintf(b, "    Interval End:        %s\n", formatTime(bar.IntervalEnd))
	fmt.Fprintf(b, "    Open:                %g\n", bar.Open)
	fmt.Fprintf(b, "    High:                %g\n", bar.High)
	fmt.Fprintf(b, "    Low:                 %g\n", bar.Low)
	fmt.Fprintf(b, "    Close:               %g\n", bar.Close)
	fmt.Fprintf(b, "    Volume:              %d\n", bar.Volume)
	fmt.Fprintf(b, "    Source:              %s\n", na(bar.Source))
	fmt.Fprintf(b, "    Source Timestamp:    %s\n", na(bar.SourceTimestamp))
	fmt.Fprintf(b, "    Quality:             %s\n", na(string(bar.QualityStatus)))
}

func writeFacts(b *strings.Builder, ev Event) {
	switch {
	case ev.Feed != nil:
		fmt.Fprintf(b, "    Source ID: %s\n", na(ev.Feed.SourceID))
		fmt.Fprintf(b, "    Last Error: %s\n", na(ev.Feed.LastError))
		if len(ev.Feed.SubscribedSymbols) > 0 {
			fmt.Fprintf(b, "    Symbols: %s\n", strings.Join(ev.Feed.SubscribedSymbols, ","))
		}
	case ev.Ingestion != nil:
		fmt.Fprintf(b, "    Feed State: %s\n", na(ev.Ingestion.FeedState))
		fmt.Fprintf(b, "    Observe: %t\n", ev.Ingestion.Observe)
		fmt.Fprintf(b, "    Infer: %t\n", ev.Ingestion.Infer)
		fmt.Fprintf(b, "    Filling: %t\n", ev.Ingestion.Filling)
		fmt.Fprintf(b, "    Source ID: %s\n", na(ev.Ingestion.SourceID))
	case ev.Adaptive != nil:
		fmt.Fprintf(b, "    Path Direction: %s\n", na(ev.Adaptive.PathDirection))
		fmt.Fprintf(b, "    Strength: %g\n", ev.Adaptive.Strength)
		fmt.Fprintf(b, "    Confidence: %g\n", ev.Adaptive.Confidence)
		fmt.Fprintf(b, "    Uncertainty: %g\n", ev.Adaptive.Uncertainty)
	case ev.Pricing != nil:
		fmt.Fprintf(b, "    Emitted: %t\n", ev.Pricing.Emitted)
	default:
		b.WriteString("    N/A\n")
	}
	b.WriteByte('\n')
}

const banner = "======================================================================"
const line = "----------------------------------------------------------------------\n"

func na(v string) string {
	if strings.TrimSpace(v) == "" {
		return "N/A"
	}
	return v
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func formatSeq(seq uint64) string {
	if seq == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d", seq)
}

func formatAccepted(seq int, hasSource bool) string {
	if seq == 0 && !hasSource {
		return "N/A"
	}
	return fmt.Sprintf("%d", seq)
}
