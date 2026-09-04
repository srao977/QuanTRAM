package pricing

// This file projects affine F4 dynamics and detects training-envelope exits.

import (
	"math"
)

const coverPoints = 11

var coverGrid = func() []float64 {
	g := make([]float64, coverPoints)
	for i := 0; i < coverPoints; i++ {
		g[i] = float64(i) / float64(coverPoints-1)
	}
	return g
}()

// coverResult contains the projected trajectory, standardized excursion,
// envelope-exit metadata, and numerical failure state.
type coverResult struct {
	Trajectory    [][3]float64
	DLocalMaximum float64
	EnvelopeExit  bool
	FirstExitTime float64
	HasFirstExit  bool
	ExitDimension string
	Unstable      bool
	Err           error
}

// analyticAffineTrajectory integrates [P,P1,P2] by exponentiating a 4x4
// homogeneous lift of the affine F4 system at each requested time.
func analyticAffineTrajectory(initial [3]float64, physical [4]float64, times []float64) ([][3]float64, error) {
	if !allFinite(initial[0], initial[1], initial[2], physical[0], physical[1], physical[2], physical[3]) {
		return nil, errf("ANALYTIC_INPUT_NONFINITE")
	}
	// 4×4 affine lift: A in [:3,:3], c in [:3,3]
	aug := make([]float64, 16)
	aug[0*4+1] = 1
	aug[1*4+2] = 1
	aug[2*4+0] = physical[1]
	aug[2*4+1] = physical[2]
	aug[2*4+2] = physical[3]
	aug[2*4+3] = physical[0]
	out := make([][3]float64, len(times))
	for i, t := range times {
		if !finite(t) {
			return nil, errf("ANALYTIC_TIME_GRID_NONFINITE")
		}
		scaled := make([]float64, 16)
		for k := 0; k < 16; k++ {
			scaled[k] = aug[k] * t
		}
		expM, ok := matrixExp(scaled, 4)
		if !ok {
			return nil, errf("ANALYTIC_NONFINITE_TRAJECTORY")
		}
		// v = [p,p1,p2,1]
		v0 := initial[0]
		v1 := initial[1]
		v2 := initial[2]
		v3 := 1.0
		out[i][0] = expM.At(0, 0)*v0 + expM.At(0, 1)*v1 + expM.At(0, 2)*v2 + expM.At(0, 3)*v3
		out[i][1] = expM.At(1, 0)*v0 + expM.At(1, 1)*v1 + expM.At(1, 2)*v2 + expM.At(1, 3)*v3
		out[i][2] = expM.At(2, 0)*v0 + expM.At(2, 1)*v1 + expM.At(2, 2)*v2 + expM.At(2, 3)*v3
		if !allFinite(out[i][0], out[i][1], out[i][2]) {
			return nil, errf("ANALYTIC_NONFINITE_TRAJECTORY")
		}
	}
	return out, nil
}

// solveCover projects over the fixed cover grid, rejects explosive terminal
// growth, and records the first departure from the fitted training envelope.
func solveCover(fit *f4Fit, p, p1, p2 float64, timeTerm bool) coverResult {
	if timeTerm {
		return coverResult{Err: errf("ANALYTIC_TIME_TERM_UNSUPPORTED")}
	}
	initial := [3]float64{p, p1, p2}
	traj, err := analyticAffineTrajectory(initial, fit.Physical, coverGrid)
	if err != nil {
		return coverResult{Err: err}
	}
	end := traj[len(traj)-1]
	scales := fit.Scales
	if math.Abs(end[0]-initial[0]) > 1e6*scales[0] ||
		math.Abs(end[1]-initial[1]) > 1e6*scales[1] ||
		math.Abs(end[2]-initial[2]) > 1e6*scales[2] {
		return coverResult{Unstable: true, Err: errf("NUMERICALLY_UNSTABLE")}
	}
	dMax := 0.0
	var firstExit float64
	hasExit := false
	exitDim := ""
	for i, pt := range traj {
		dx := (pt[0] - fit.Means[0]) / scales[0]
		dy := (pt[1] - fit.Means[1]) / scales[1]
		dz := (pt[2] - fit.Means[2]) / scales[2]
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > dMax {
			dMax = dist
		}
		inside := [3]bool{
			pt[0] >= fit.Minimum[0] && pt[0] <= fit.Maximum[0],
			pt[1] >= fit.Minimum[1] && pt[1] <= fit.Maximum[1],
			pt[2] >= fit.Minimum[2] && pt[2] <= fit.Maximum[2],
		}
		if inside[0] && inside[1] && inside[2] {
			continue
		}
		if !hasExit {
			hasExit = true
			firstExit = coverGrid[i]
			names := []string{"P", "P1", "P2"}
			var parts []string
			for d := 0; d < 3; d++ {
				if !inside[d] {
					parts = append(parts, names[d])
				}
			}
			exitDim = joinPipe(parts)
		}
	}
	return coverResult{
		Trajectory:    traj,
		DLocalMaximum: dMax,
		EnvelopeExit:  hasExit,
		FirstExitTime: firstExit,
		HasFirstExit:  hasExit,
		ExitDimension: exitDim,
	}
}

func joinPipe(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "|" + parts[i]
	}
	return out
}
