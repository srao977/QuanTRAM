package adaptive

import "math"

func updateVolumeInfluence(volume, prevReference float64, cfg VolumeConfig, epsilon float64) (float64, float64) {
	ref := (1.0-cfg.ReferenceAlpha)*prevReference + cfg.ReferenceAlpha*volume
	relative := math.Log1p(volume / max(ref, epsilon))
	absolute := math.Log1p(max(volume, 0.0)) / 10.0
	vStar := cfg.Influence.Clamp(relative + absolute)
	return ref, vStar
}
