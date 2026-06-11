# wifitune

Probe a WiFi adapter's signal strength and turn it into adaptive beeps for
antenna tuning. Point your router's antennas or walk around with your laptop to
find the best reception spot — as signal improves the pause between beeps shrinks
and the pitch rises; at the strongest spot the tone goes solid. A live terminal
dashboard shows the numbers and three graphs.

## How it works

- **Signal source** — reads dBm directly from the kernel via netlink (nl80211).
  No shelling out, no `iw` parsing. Works without root.
- **Audio** — pure-Go PulseAudio client generates a sine wave on the fly. The
  beep gap and pitch are recomputed every cycle from the latest reading, so the
  cadence adapts in real time.
- **UI** — a Bubble Tea dashboard with a bar, a recent sparkline, and a
  full-session compressed graph.

The whole binary is pure Go (`CGO_ENABLED=0`), so it cross-compiles to any
Linux `GOARCH` (amd64, arm64, arm, …) with a single `go build`.

## Platform support

**Linux only.** Signal reading uses nl80211 netlink (a Linux kernel interface)
and audio uses the PulseAudio protocol — neither exists on macOS or Windows.

| Platform | Status |
|----------|--------|
| Linux    | supported |
| macOS    | not supported — would need a CoreWLAN signal backend and a non-Pulse audio backend |
| Windows  | not supported — would need a Native WiFi (`wlanapi`/`netsh`) signal backend and a WASAPI audio backend |

The source files carry a `//go:build linux` constraint, so building for
`GOOS=darwin` or `GOOS=windows` fails immediately rather than producing a binary
that launches and then errors at runtime.

## Signal scale

WiFi signal is measured in **dBm** (decibel-milliwatts), always negative.
More negative = weaker.

| dBm    | meaning            |
|--------|--------------------|
| -30    | excellent (next to router) — treated as 100% |
| -67    | good, reliable     |
| -70    | usable             |
| -80    | weak               |
| -90    | unusable — treated as 0% |

There is no true "100%". The tool maps the dBm range onto 0–100% using fixed
endpoints (`-min -90`, `-max -30` by default), which you can override.

## Build

```sh
go build -o wifitune .
```

Cross-compile to another Linux architecture (example: ARM64, e.g. a Raspberry Pi):

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o wifitune .
```

## Usage

```sh
./wifitune <interface>
```

Find your wireless interface name:

```sh
./wifitune --list
```

Example:

```sh
./wifitune wlp0s20f3
```

Turn an antenna slowly, or walk around with your laptop to find the sweet spot in
your space. The beep gap shrinks and pitch rises as signal climbs; a solid
continuous tone means you've hit the configured ceiling — that's the spot. Press
`q` (or `Ctrl-C`) to quit.

### Dashboard

```
WiFi Antenna Tuner  wlp0s20f3

-52 dBm   63%   raw -54
[██████████████░░░░░░░░]

recent (last 60)
▁▁▂▃▃▄▅▅▆▇▇█▇▆▅▄▃

session (1430 samples)
start -78dBm/20% ▁▂▂▃▄▄▅▆▆▇▇█ -52dBm/63% now

beep: 440ms gap   pitch: 760 Hz
q to quit
```

1. **bar** — current signal percentage.
2. **recent** — sparkline of the last 60 samples.
3. **session** — the entire run compressed to a fixed width (older samples are
   bucket-averaged so it always fits). The starting value is labelled on the
   left, the current value on the right — see your whole tuning progress at a
   glance.

## Flags

| Flag         | Default | Description |
|--------------|---------|-------------|
| `--min`      | `-90`   | dBm floor, maps to 0% |
| `--max`      | `-30`   | dBm ceiling, maps to 100% |
| `--interval` | `300ms` | poll interval |
| `--ema`      | `0.4`   | smoothing factor 0..1 (higher = snappier, less smoothing) |
| `--freq-min` | `400`   | beep pitch in Hz at 0% |
| `--freq-max` | `1200`  | beep pitch in Hz at 100% |
| `--max-gap`  | `1200`  | pause in ms between beeps at 0% |
| `--solid`    | `95`    | percent at/above which the tone is continuous |
| `--no-sound` | `false` | disable audio (visual only) |
| `--no-ui`    | `false` | disable the dashboard (headless, line-by-line) |
| `--list`     | `false` | list wireless interfaces and exit |

### Tips

- If your room never reaches the "solid" threshold, lower the ceiling so it's
  reachable: `./wifitune wlp0s20f3 --max -45`.
- For a steadier beep cadence, lower `--ema` (e.g. `0.2`); for snappier
  response while turning the antenna, raise it.

## Requirements

- Linux (signal reading uses nl80211 netlink).
- A running PulseAudio or PipeWire (with the PulseAudio shim) daemon for sound.
  Without it the tool prints a warning and continues silently — use `--no-sound`
  to silence the warning.

## Dependencies

- [`github.com/mdlayher/wifi`](https://github.com/mdlayher/wifi) — nl80211 signal reading
- [`github.com/jfreymuth/pulse`](https://github.com/jfreymuth/pulse) — pure-Go PulseAudio playback
- [`github.com/charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) + [`lipgloss`](https://github.com/charmbracelet/lipgloss) — terminal UI

## Credits

Developed by [Gudasoft](https://gudasoft.com/products/wifi-tune).
