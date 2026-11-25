package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/sysstats"
)

// The chrome every full screen shares, as the unattended run drew it first:
//
//	 Ω omega  what this screen is  version · model · flags      right
//	 CPU ━━━━━──── 23%   RAM ━━━━━━─── 62% 10/16 GB   / ━━━━━━── 80%   NET ↓1 MB/s ↑40 KB/s
//	 …the screen…
//	────────────────────────────────────────────────────────────────────
//	  key action · key action                                      note
//
// The second line is the machine, measured once a second while a screen
// is up.

// statsNow is the sample the screens draw; tests pin it.
var statsNow = sysstats.Latest

// watchStats starts measuring the machine for as long as a screen is up.
var watchStats = sysstats.Watch

// statsTickMsg redraws a screen that has no animation of its own, so the
// figures move.
type statsTickMsg struct{}

func statsTick() tea.Cmd {
	return tea.Tick(sysstats.Interval, func(time.Time) tea.Msg { return statsTickMsg{} })
}

// topBar is a screen's first line: the brand, what the screen is, what it
// runs with, and on the right what it has taken so far. The details go
// first when there is no room, then the title is cut.
func topBar(title string, info []string, right string, w int) string {
	if right != "" {
		right += " "
	}
	left := " " + titleStyle.Render(gl.Brand)
	if title != "" {
		left += "  " + cmdStyle.Render(title)
	}
	for len(info) > 0 {
		more := faintStyle.Render("  " + strings.Join(info, " "+gl.Sep+" "))
		if lipgloss.Width(left)+lipgloss.Width(more)+lipgloss.Width(right)+2 <= w {
			left += more
			break
		}
		info = info[1:]
	}
	return spread(left, right, w)
}

// rightInfo joins what goes on the right of the top bar.
func rightInfo(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, mutedStyle.Render(p))
		}
	}
	return strings.Join(keep, faintStyle.Render(" "+gl.Sep+" "))
}

// chromeFooter is the bottom of a screen: a rule, then the keys, with a
// note on the right when there is room for it.
func chromeFooter(w int, note string, keys ...hint) []string {
	left := strings.Split(hintsAt(w-1, " ", true, keys...), "\n")
	if note != "" && lipgloss.Width(left[0])+lipgloss.Width(note)+4 <= w {
		left[0] = spread(left[0], note+" ", w)
	}
	return append([]string{chromeRule(w)}, left...)
}

// chromeRule is the faint rule above a screen's keys.
func chromeRule(w int) string { return faintStyle.Render(strings.Repeat(gl.Rule, max(0, w))) }

// statsLine is the machine on one line, w columns wide: the processor, the
// memory, every disk and the network. On a narrow terminal the bars get
// shorter, then the amounts, the other disks and the bars go.
func statsLine(w int) string {
	s := statsNow()
	room := w - 2
	var last string
	for _, l := range statsLevels {
		line := " " + strings.Join(statsParts(s, l), "   ")
		last = line
		if lipgloss.Width(line) <= room+1 {
			return line
		}
	}
	return truncateANSI(last, w)
}

// statsLevel is how much the stats line shows.
type statsLevel struct {
	bar     int  // width of each bar; 0 for none
	amounts bool // the memory in bytes, beside its share
	disks   int  // how many disks; -1 for all
	names   bool // the name of each disk
}

var statsLevels = []statsLevel{
	{bar: 10, amounts: true, disks: -1, names: true},
	{bar: 6, amounts: true, disks: -1, names: true},
	{bar: 6, amounts: false, disks: -1, names: true},
	{bar: 6, amounts: false, disks: 1, names: true},
	{bar: 4, amounts: false, disks: 1, names: false},
	{bar: 0, amounts: false, disks: 1, names: false},
}

func statsParts(s sysstats.Sample, l statsLevel) []string {
	label := func(key string) string { return faintStyle.Render(i18n.T(key)) + " " }
	share := func(v float64, known bool) string {
		if !known {
			if l.bar > 0 {
				return faintStyle.Render(strings.Repeat(gl.Rule, l.bar)+" ") + faintStyle.Render(gl.Ellipsis)
			}
			return faintStyle.Render(gl.Ellipsis)
		}
		pct := cmdStyle.Render(fmt.Sprintf("%3.0f%%", v*100))
		if l.bar > 0 {
			return gauge(v, l.bar) + " " + pct
		}
		return pct
	}
	var parts []string
	parts = append(parts, label("stats.cpu")+share(s.CPU, s.CPUKnown))
	ram := label("stats.ram") + share(s.MemShare(), s.MemKnown)
	if l.amounts && s.MemKnown {
		ram += " " + mutedStyle.Render(memAmount(s.MemUsed, s.MemTotal))
	}
	parts = append(parts, ram)
	disks := s.Disks
	if l.disks >= 0 && len(disks) > l.disks {
		disks = disks[:l.disks]
	}
	if len(disks) == 0 {
		parts = append(parts, label("stats.disk")+share(0, false))
	}
	for i, d := range disks {
		// DISK / ━━━ 80%   Backup ━━━ 12%: the label once, the names
		// when there is more than one disk.
		name := ""
		if i == 0 {
			name = label("stats.disk")
		}
		if l.names && len(s.Disks) > 1 {
			name += mutedStyle.Render(diskName(d.Mount)) + " "
		}
		parts = append(parts, name+share(d.UsedShare(), true))
	}
	nw := label("stats.net")
	if s.NetKnown {
		nw += okStyle.Render(gl.Down) + cmdStyle.Render(rateText(s.RxRate)) + " " + accentStyle.Render(gl.Up) + cmdStyle.Render(rateText(s.TxRate))
	} else {
		nw += faintStyle.Render(gl.Ellipsis)
	}
	return append(parts, nw)
}

// diskName is how the stats line names a volume: "/" for the system,
// the last part of the path for the rest.
func diskName(mount string) string {
	if mount == "/" || mount == "" {
		return "/"
	}
	if len(mount) <= 3 && strings.HasSuffix(strings.TrimRight(mount, `\`), ":") {
		return strings.TrimRight(mount, `\`) // C:
	}
	return truncate(filepath.Base(mount), 14)
}

// memAmount is the memory in use of the total: "10.2/16 GB".
func memAmount(used, total uint64) string {
	gb := func(v uint64) string {
		x := float64(v) / (1 << 30)
		s := fmt.Sprintf("%.1f", x)
		if x >= 100 || x == float64(int(x)) {
			s = fmt.Sprintf("%.0f", x)
		}
		if decimalComma() {
			s = strings.Replace(s, ".", ",", 1)
		}
		return s
	}
	return gb(used) + "/" + gb(total) + " GB"
}

// rateText is a transfer rate, short: "1.2 MB/s", "40 KB/s", "0 B/s".
func rateText(bps float64) string {
	var t string
	switch {
	case bps < 1<<20:
		return fmt.Sprintf("%.0f KB/s", bps/(1<<10))
	case bps < 10<<20:
		t = fmt.Sprintf("%.1f MB/s", bps/(1<<20))
	default:
		return Bytes(int64(bps)) + "/s"
	}
	if decimalComma() {
		t = strings.Replace(t, ".", ",", 1)
	}
	return t
}
