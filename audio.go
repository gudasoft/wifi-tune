//go:build linux

package main

import (
	"context"
	"math"
	"sync/atomic"

	"github.com/jfreymuth/pulse"
)

const (
	sampleRate = 44100
	amplitude  = 0.20 // keep it gentle on the ears
	rampMs     = 4.0  // attack/release to avoid clicks
)

// AudioBeeper plays a continuous PulseAudio stream whose cadence and pitch are
// driven by the shared signal percentage. Pure-Go: jfreymuth/pulse speaks the
// PulseAudio native protocol over its unix socket, no CGo.
type AudioBeeper struct {
	client *pulse.Client
}

// NewAudioBeeper connects to the PulseAudio (or PipeWire pulse shim) daemon.
func NewAudioBeeper() (*AudioBeeper, error) {
	c, err := pulse.NewClient()
	if err != nil {
		return nil, err
	}
	return &AudioBeeper{client: c}, nil
}

func (a *AudioBeeper) Close() { a.client.Close() }

// Run drives a gapless playback stream. The reader callback regenerates its
// per-cycle parameters (frequency, beep length, gap length) at each cycle
// boundary by reading the shared percentage, so cadence adapts in real time.
func (a *AudioBeeper) Run(ctx context.Context, cfg Config, sharedPct *atomic.Value) error {
	rampSamples := int(math.Round(rampMs / 1000 * sampleRate))

	// Per-cycle state, persisted across reader calls.
	var (
		phase      float64 // sine phase in radians
		posInCycle int
		beepLen    int
		gapLen     int
		freq       float64
		solid      bool
		started    bool
	)

	recompute := func() {
		pct := 0.0
		if v := sharedPct.Load(); v != nil {
			pct = v.(float64)
		}
		freq = cfg.Freq(pct)
		solid = cfg.Solid(pct)
		beepLen = int(cfg.BeepMs / 1000 * sampleRate)
		if solid {
			gapLen = 0
		} else {
			gapLen = int(cfg.GapMs(pct) / 1000 * sampleRate)
		}
		if beepLen < 1 {
			beepLen = 1
		}
		phase = 0
		posInCycle = 0
	}

	reader := func(out []float32) (int, error) {
		select {
		case <-ctx.Done():
			return 0, pulse.EndOfData
		default:
		}
		for i := range out {
			if !started || posInCycle >= beepLen+gapLen {
				recompute()
				started = true
			}
			var s float64
			if posInCycle < beepLen {
				s = amplitude * math.Sin(phase)
				phase += 2 * math.Pi * freq / sampleRate
				// Apply attack/release envelope at beep edges.
				if posInCycle < rampSamples {
					s *= float64(posInCycle) / float64(rampSamples)
				} else if !solid && posInCycle > beepLen-rampSamples {
					s *= float64(beepLen-posInCycle) / float64(rampSamples)
				}
			}
			out[i] = float32(s)
			posInCycle++
		}
		return len(out), nil
	}

	stream, err := a.client.NewPlayback(pulse.Float32Reader(reader), pulse.PlaybackLatency(0.05))
	if err != nil {
		return err
	}
	defer stream.Close()

	stream.Start()
	<-ctx.Done()
	stream.Stop()
	return nil
}
