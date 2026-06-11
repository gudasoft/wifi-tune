//go:build linux

package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/mdlayher/wifi"
)

// Sample is one smoothed signal reading pushed to the UI.
type Sample struct {
	DBmRaw    float64
	DBmSmooth float64
	Percent   float64
	Err       error
}

// SignalReader reads dBm from a named interface via nl80211 (netlink).
type SignalReader struct {
	client *wifi.Client
	ifi    *wifi.Interface
}

// NewSignalReader opens a netlink client and locates the named interface.
func NewSignalReader(name string) (*SignalReader, error) {
	c, err := wifi.New()
	if err != nil {
		return nil, fmt.Errorf("open nl80211: %w", err)
	}
	ifis, err := c.Interfaces()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	for _, ifi := range ifis {
		if ifi.Name == name {
			return &SignalReader{client: c, ifi: ifi}, nil
		}
	}
	c.Close()
	return nil, fmt.Errorf("interface %q not found (use --list to see wireless interfaces)", name)
}

// Close releases the netlink client.
func (s *SignalReader) Close() error { return s.client.Close() }

// read returns the current signal in dBm. When connected to an AP the station
// info carries the per-link signal strength.
func (s *SignalReader) read() (float64, error) {
	stations, err := s.client.StationInfo(s.ifi)
	if err != nil {
		return 0, fmt.Errorf("station info: %w (interface may be down or not connected)", err)
	}
	if len(stations) == 0 {
		return 0, fmt.Errorf("no station info: %s not associated with an access point", s.ifi.Name)
	}
	// Pick the strongest station (usually exactly one for a client interface).
	best := float64(stations[0].Signal)
	for _, st := range stations[1:] {
		if float64(st.Signal) > best {
			best = float64(st.Signal)
		}
	}
	return best, nil
}

// Run polls the interface on the configured interval, applies exponential
// smoothing, publishes each Sample on out, and stores the smoothed dBm in the
// shared atomic for the audio loop. It stops when ctx is cancelled.
func (s *SignalReader) Run(ctx context.Context, cfg Config, interval time.Duration, ema float64, out chan<- Sample, sharedPct *atomic.Value) {
	t := time.NewTicker(interval)
	defer t.Stop()

	var smooth float64
	first := true

	emit := func() {
		raw, err := s.read()
		if err != nil {
			select {
			case out <- Sample{Err: err}:
			case <-ctx.Done():
			}
			return
		}
		if first {
			smooth = raw
			first = false
		} else {
			smooth = ema*raw + (1-ema)*smooth
		}
		pct := cfg.Percent(smooth)
		sharedPct.Store(pct)
		select {
		case out <- Sample{DBmRaw: raw, DBmSmooth: smooth, Percent: pct}:
		case <-ctx.Done():
		}
	}

	emit() // immediate first reading
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			emit()
		}
	}
}

// ListInterfaces returns the names of all wireless interfaces on the system.
func ListInterfaces() ([]string, error) {
	c, err := wifi.New()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	ifis, err := c.Interfaces()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ifi := range ifis {
		if ifi.Name != "" {
			names = append(names, ifi.Name)
		}
	}
	return names, nil
}
