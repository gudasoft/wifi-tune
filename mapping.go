package main

import "math"

// Config holds the tunable mapping parameters derived from CLI flags.
type Config struct {
	MinDBm     float64 // signal floor, maps to 0%
	MaxDBm     float64 // signal ceiling, maps to 100%
	FreqMin    float64 // pitch in Hz at 0%
	FreqMax    float64 // pitch in Hz at 100%
	MaxGapMs   float64 // pause between beeps at 0%
	BeepMs     float64 // duration of a single beep tone
	SolidAbove float64 // percent at/above which tone becomes continuous
}

// DefaultConfig returns sensible defaults; flags override individual fields.
func DefaultConfig() Config {
	return Config{
		MinDBm:     -90,
		MaxDBm:     -30,
		FreqMin:    400,
		FreqMax:    1200,
		MaxGapMs:   1200,
		BeepMs:     120,
		SolidAbove: 95,
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// Percent maps a dBm reading onto 0..100 using the configured endpoints.
func (c Config) Percent(dBm float64) float64 {
	if c.MaxDBm == c.MinDBm {
		return 0
	}
	t := (dBm - c.MinDBm) / (c.MaxDBm - c.MinDBm)
	return clamp(t, 0, 1) * 100
}

// Freq returns the beep pitch in Hz for a given percent.
func (c Config) Freq(pct float64) float64 {
	return lerp(c.FreqMin, c.FreqMax, pct/100)
}

// GapMs returns the pause between beeps in milliseconds. Above SolidAbove the
// gap collapses to zero, yielding a continuous tone.
func (c Config) GapMs(pct float64) float64 {
	if pct >= c.SolidAbove {
		return 0
	}
	// Scale gap within the 0..SolidAbove band so it reaches 0 at the threshold.
	t := pct / c.SolidAbove
	return lerp(c.MaxGapMs, 0, t)
}

// Solid reports whether the tone should be continuous (no gaps).
func (c Config) Solid(pct float64) bool { return pct >= c.SolidAbove }

// round helper for display.
func roundTo(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
