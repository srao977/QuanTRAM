package pricing

// This file assembles fitted dynamics and projected cover data for policy use.

import "math"

// numericalRow is the internal numerical contract between fitting, projection,
// emission policy, and cockpit interpretation.
type numericalRow struct {
	Index                                int
	ObservationIndex                     int
	Symbol                               string
	Timestamp                            string
	Session                              string
	Open, High, Low, Close               float64
	Volume                               float64
	SourceProvider                       string
	P, P1, P2                            float64
	ProjectedP, ProjectedP1, ProjectedP2 float64
	RKSuccess                            bool
	DomainExit                           bool
	ConditionNumber                      float64
	MaxRealEigenvalue                    float64
	PerturbationAmplification            float64
	DLocalMaximum                        float64
	HasDLocal                            bool
	FirstExitTime                        float64
	HasFirstExit                         bool
	ExitDimension                        string
}

// buildNumericalRow derives local stability diagnostics from the F4 companion
// matrix. Failed or unstable covers preserve the input state as the projection.
func buildNumericalRow(obs Observation, index, globalActive int, fit *f4Fit, cover coverResult, p, p1, p2 float64) numericalRow {
	companion := []float64{
		0, 1, 0,
		0, 0, 1,
		fit.Physical[1], fit.Physical[2], fit.Physical[3],
	}
	eig, eigOK := maxRealEigenvalue(companion, 3)
	if !eigOK {
		eig = math.NaN()
	}
	amp := math.NaN()
	if expA, ok := matrixExp(companion, 3); ok {
		amp = maxColumnNorm2(expA, 3, 3)
	}

	row := numericalRow{
		Index:                     globalActive,
		ObservationIndex:          globalActive + 1,
		Symbol:                    obs.Entity,
		Timestamp:                 obs.Timestamp,
		Session:                   obs.Session,
		Open:                      obs.Open,
		High:                      obs.High,
		Low:                       obs.Low,
		Close:                     obs.Close,
		Volume:                    obs.Volume,
		SourceProvider:            obs.Source,
		P:                         p,
		P1:                        p1,
		P2:                        p2,
		ConditionNumber:           fit.Condition,
		MaxRealEigenvalue:         eig,
		PerturbationAmplification: amp,
	}
	if cover.Err != nil || cover.Unstable || len(cover.Trajectory) == 0 {
		row.ProjectedP = p
		row.ProjectedP1 = p1
		row.ProjectedP2 = p2
		row.RKSuccess = false
		row.DomainExit = false
		return row
	}
	end := cover.Trajectory[len(cover.Trajectory)-1]
	row.ProjectedP = end[0]
	row.ProjectedP1 = end[1]
	row.ProjectedP2 = end[2]
	row.RKSuccess = true
	row.DomainExit = cover.EnvelopeExit
	row.DLocalMaximum = cover.DLocalMaximum
	row.HasDLocal = true
	row.FirstExitTime = cover.FirstExitTime
	row.HasFirstExit = cover.HasFirstExit
	row.ExitDimension = cover.ExitDimension
	return row
}
