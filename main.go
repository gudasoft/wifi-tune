package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg := DefaultConfig()

	var (
		interval = flag.Duration("interval", 300*time.Millisecond, "poll interval")
		ema      = flag.Float64("ema", 0.4, "smoothing factor 0..1 (higher = snappier)")
		noSound  = flag.Bool("no-sound", false, "disable audio feedback")
		noUI     = flag.Bool("no-ui", false, "disable terminal UI (headless)")
		list     = flag.Bool("list", false, "list wireless interfaces and exit")
	)
	flag.Float64Var(&cfg.MinDBm, "min", cfg.MinDBm, "dBm floor (maps to 0%)")
	flag.Float64Var(&cfg.MaxDBm, "max", cfg.MaxDBm, "dBm ceiling (maps to 100%)")
	flag.Float64Var(&cfg.FreqMin, "freq-min", cfg.FreqMin, "beep pitch Hz at 0%")
	flag.Float64Var(&cfg.FreqMax, "freq-max", cfg.FreqMax, "beep pitch Hz at 100%")
	flag.Float64Var(&cfg.MaxGapMs, "max-gap", cfg.MaxGapMs, "pause ms between beeps at 0%")
	flag.Float64Var(&cfg.SolidAbove, "solid", cfg.SolidAbove, "percent at/above which tone is continuous")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: wifitune [flags] <interface>\n\n")
		fmt.Fprintf(os.Stderr, "Probe WiFi signal and turn it into adaptive beeps for antenna tuning.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *list {
		names, err := ListInterfaces()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Println("no wireless interfaces found")
			return
		}
		fmt.Println("wireless interfaces:")
		for _, n := range names {
			fmt.Println("  " + n)
		}
		return
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	iface := flag.Arg(0)

	reader, err := NewSignalReader(iface)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer reader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ctrl-C / SIGTERM cancels everything.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()

	var sharedPct atomic.Value
	sharedPct.Store(0.0)
	samples := make(chan Sample, 8)

	// Poller goroutine.
	go reader.Run(ctx, cfg, *interval, *ema, samples, &sharedPct)

	// Audio goroutine (optional).
	if !*noSound {
		beeper, err := NewAudioBeeper()
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: audio unavailable (", err, ") — continuing without sound")
		} else {
			defer beeper.Close()
			go func() {
				if err := beeper.Run(ctx, cfg, &sharedPct); err != nil {
					fmt.Fprintln(os.Stderr, "audio stopped:", err)
				}
			}()
		}
	}

	if *noUI {
		runHeadless(ctx, samples)
		return
	}

	p := tea.NewProgram(newModel(iface, cfg, samples))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ui error:", err)
	}
	cancel()
}

// runHeadless prints readings line-by-line until the context is cancelled.
func runHeadless(ctx context.Context, samples <-chan Sample) {
	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-samples:
			if !ok {
				return
			}
			if s.Err != nil {
				fmt.Println("error:", s.Err)
				continue
			}
			fmt.Printf("%.0f dBm  %.0f%%  (raw %.0f)\n", roundTo(s.DBmSmooth, 0), s.Percent, s.DBmRaw)
		}
	}
}
