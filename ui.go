//go:build linux

package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const histLen = 60

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

type sampleMsg Sample

// waitForSample yields a tea.Cmd that blocks on the next signal sample.
func waitForSample(sub <-chan Sample) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-sub
		if !ok {
			return tea.Quit()
		}
		return sampleMsg(s)
	}
}

type model struct {
	cfg      Config
	iface    string
	sub      <-chan Sample
	last     Sample
	history  []float64 // recent window (sparkline #2)
	full     []float64 // entire session, percent (compressed graph #3)
	startDBm float64
	startPct float64
	hasStart bool
	width    int
	hasData  bool
}

func newModel(iface string, cfg Config, sub <-chan Sample) model {
	return model{cfg: cfg, iface: iface, sub: sub, width: 40}
}

func (m model) Init() tea.Cmd { return waitForSample(m.sub) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case sampleMsg:
		s := Sample(msg)
		m.last = s
		m.hasData = true
		if s.Err == nil {
			if !m.hasStart {
				m.startDBm = s.DBmSmooth
				m.startPct = s.Percent
				m.hasStart = true
			}
			m.history = append(m.history, s.Percent)
			if len(m.history) > histLen {
				m.history = m.history[len(m.history)-histLen:]
			}
			m.full = append(m.full, s.Percent)
		}
		return m, waitForSample(m.sub)
	}
	return m, nil
}

func colorFor(pct float64) lipgloss.Color {
	switch {
	case pct >= 66:
		return lipgloss.Color("10") // green
	case pct >= 33:
		return lipgloss.Color("11") // yellow
	default:
		return lipgloss.Color("9") // red
	}
}

func bar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func sparkline(hist []float64) string {
	if len(hist) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range hist {
		idx := int(p / 100 * float64(len(sparkRunes)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkRunes) {
			idx = len(sparkRunes) - 1
		}
		b.WriteRune(sparkRunes[idx])
	}
	return b.String()
}

// compressSparkline downsamples the full session history into cols columns by
// averaging each bucket, so the whole session always fits a fixed width.
func compressSparkline(hist []float64, cols int) string {
	if len(hist) == 0 || cols < 1 {
		return ""
	}
	if len(hist) <= cols {
		return sparkline(hist)
	}
	out := make([]float64, cols)
	for i := 0; i < cols; i++ {
		lo := i * len(hist) / cols
		hi := (i + 1) * len(hist) / cols
		if hi <= lo {
			hi = lo + 1
		}
		var sum float64
		for _, v := range hist[lo:hi] {
			sum += v
		}
		out[i] = sum / float64(hi-lo)
	}
	return sparkline(out)
}

func (m model) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("WiFi Antenna Tuner") + "  " + dimStyle.Render(m.iface) + "\n\n")

	if !m.hasData {
		b.WriteString(dimStyle.Render("waiting for signal…") + "\n")
		return b.String()
	}

	if m.last.Err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
		b.WriteString(errStyle.Render("error: "+m.last.Err.Error()) + "\n")
		b.WriteString(dimStyle.Render("press q to quit") + "\n")
		return b.String()
	}

	pct := m.last.Percent
	c := colorFor(pct)
	big := lipgloss.NewStyle().Bold(true).Foreground(c)

	gap := m.cfg.GapMs(pct)
	freq := m.cfg.Freq(pct)
	cadence := fmt.Sprintf("%.0fms gap", gap)
	if m.cfg.Solid(pct) {
		cadence = "SOLID (locked)"
	}

	b.WriteString(fmt.Sprintf("%s   %s   %s\n",
		big.Render(fmt.Sprintf("%.0f dBm", roundTo(m.last.DBmSmooth, 0))),
		big.Render(fmt.Sprintf("%.0f%%", pct)),
		dimStyle.Render(fmt.Sprintf("raw %.0f", m.last.DBmRaw)),
	))

	barWidth := m.width - 4
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 60 {
		barWidth = 60
	}
	b.WriteString(lipgloss.NewStyle().Foreground(c).Render("["+bar(pct, barWidth)+"]") + "\n\n")

	b.WriteString(dimStyle.Render(fmt.Sprintf("recent (last %d)", histLen)) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(c).Render(sparkline(m.history)) + "\n\n")

	// Graph #3: entire session compressed to a fixed width, start value on the
	// left, current value on the right.
	sparkW := barWidth
	startLbl := fmt.Sprintf("%.0fdBm/%.0f%%", roundTo(m.startDBm, 0), m.startPct)
	nowLbl := fmt.Sprintf("%.0fdBm/%.0f%%", roundTo(m.last.DBmSmooth, 0), pct)
	b.WriteString(dimStyle.Render(fmt.Sprintf("session (%d samples)", len(m.full))) + "\n")
	b.WriteString(fmt.Sprintf("%s %s %s\n",
		dimStyle.Render("start "+startLbl),
		lipgloss.NewStyle().Foreground(c).Render(compressSparkline(m.full, sparkW)),
		dimStyle.Render(nowLbl+" now"),
	))
	b.WriteString("\n")

	b.WriteString(dimStyle.Render(fmt.Sprintf("beep: %s   pitch: %.0f Hz", cadence, freq)) + "\n")
	b.WriteString(dimStyle.Render("q to quit") + "\n")
	return b.String()
}
