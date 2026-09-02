package pricing

import "testing"

func TestObservationMinutesConsecutive(t *testing.T) {
	cfg := DefaultConfig("AAPL")
	a, err := BarFromOHLCV("AAPL", "2022-09-30 04:00:00", 1, 2, 0.5, 1.5, 10)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BarFromOHLCV("AAPL", "2022-09-30 04:01:00", 1, 2, 0.5, 1.6, 10)
	if err != nil {
		t.Fatal(err)
	}
	oa := ObservationFromBar(a, cfg)
	ob := ObservationFromBar(b, cfg)
	if diff := ob.Minutes - oa.Minutes; diff != 1 {
		t.Fatalf("minute delta %v want 1", diff)
	}
}

func TestSolveCoverRejectsTimeTerm(t *testing.T) {
	fit := &f4Fit{Physical: [4]float64{0, 0, 0, 0}, Scales: [3]float64{1, 1, 1}}
	res := solveCover(fit, 1, 0, 0, true)
	if res.Err == nil || res.Err.Error() != "ANALYTIC_TIME_TERM_UNSUPPORTED" {
		t.Fatalf("err=%v", res.Err)
	}
}
