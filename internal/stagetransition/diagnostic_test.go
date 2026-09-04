package stagetransition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quantram/internal/domain"
)

func TestDiagnosticWritesHumanBlockAndLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stage_transitions.txt")
	h := NewHub()
	diag, err := NewDiagnostic(h, path)
	if err != nil {
		t.Fatal(err)
	}

	bar := time.Date(2026, 9, 4, 13, 31, 0, 0, time.UTC)
	decide(h, "SPY", domain.SideHold, bar, "spy:1")
	decide(h, "SPY", domain.SideHold, bar.Add(time.Minute), "spy:2")
	decide(h, "SPY", domain.SideBuy, bar.Add(2*time.Minute), "spy:3")
	priceOut(h, "SPY", domain.PricingStatusEmitted, "GREEN", bar, "px:1")

	deadline := time.Now().Add(2 * time.Second)
	for diag.Written() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if diag.Written() != 3 {
		t.Fatalf("written %d", diag.Written())
	}
	diag.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, need := range []string{
		"QUANTRAM STAGE TRANSITION DIAGNOSTIC",
		"Contract Version:   1.1",
		"STAGE TRANSITION",
		"Transition ID:      P03_ADAPTIVE:SPY:1",
		"Sequence:           1",
		"Stage ID:           P03_ADAPTIVE",
		"Stage Name:         Adaptive Model",
		"Entity:             SPY",
		"Previous State:",
		"ABSENT",
		"Current State:",
		"DECISION:HOLD",
		"Effective Time:",
		"Published Time:",
		"Market Snapshot ID:",
		"Accepted Sequence:",
		"Source Event ID:    spy:1",
		"Initiating Bar:",
		"Symbol:              SPY",
		"Open:",
		"High:",
		"Low:",
		"Close:",
		"Volume:",
		"Transition Facts:",
		"Correlation:",
		"DECISION:BUY",
		"P04_PRICE_ENGINE",
		"Price Engine",
		"EMITTED:GREEN",
		"STAGE TRANSITION DIAGNOSTIC COMPLETE",
		"Transitions Written:  3",
	} {
		if !strings.Contains(text, need) {
			t.Fatalf("missing %q in\n%s", need, text)
		}
	}
	if strings.Count(text, "STAGE TRANSITION\n") != 3 {
		t.Fatalf("same-state leaked extra blocks: %d", strings.Count(text, "STAGE TRANSITION\n"))
	}
}

func TestDiagnosticFailureDoesNotFailPublisher(t *testing.T) {
	h := NewHub()
	_, err := NewDiagnostic(h, filepath.Join(t.TempDir(), "no", "such", "dir", "out.txt"))
	if err == nil {
		t.Fatal("want open error")
	}
	id, ch := h.Subscribe(2)
	defer h.Unsubscribe(id)
	decide(h, "AAPL", domain.SideHold, time.Now().UTC().Truncate(time.Minute), "a:1")
	_ = drain(t, ch, 1)
}

func TestDiagnosticCloseIdempotent(t *testing.T) {
	h := NewHub()
	diag, err := NewDiagnostic(h, filepath.Join(t.TempDir(), "d.txt"))
	if err != nil {
		t.Fatal(err)
	}
	diag.Close()
	diag.Close()
}

func TestDiagnosticNonBarShowsInitiatingNA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p01.txt")
	h := NewHub()
	diag, err := NewDiagnostic(h, path)
	if err != nil {
		t.Fatal(err)
	}
	h.OnFeed(domain.FeedHealth{State: domain.FeedFailed, SourceID: "ALPACA_IEX"})
	deadline := time.Now().Add(2 * time.Second)
	for diag.Written() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	diag.Close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "Initiating Bar:\n    N/A") {
		t.Fatalf("want Initiating Bar N/A\n%s", text)
	}
}

func TestDiagnosticDoesNotWriteRepoRoot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("go.mod not found")
		}
		root = parent
	}
	if _, err := os.Stat(filepath.Join(root, "stage_transitions.txt")); err == nil {
		t.Fatal("unit tests must not create repository-root stage_transitions.txt")
	}
}

func TestSlowDiagnosticDoesNotBlockHub(t *testing.T) {
	h := NewHub()
	id, _ := h.Subscribe(1)
	defer h.Unsubscribe(id)
	decide(h, "AAPL", domain.SideHold, time.Now().UTC().Truncate(time.Minute), "a:1")
	done := make(chan struct{})
	go func() {
		decide(h, "AAPL", domain.SideBuy, time.Now().UTC().Truncate(time.Minute), "a:2")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("hub blocked")
	}
}
