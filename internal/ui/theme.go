// Package ui implements the Bubble Tea interface of todo.
package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette taken from the design mockup.
var (
	cBorder = lipgloss.Color("#3b3b52")
	cViolet = lipgloss.Color("#7d56f4")
	cLilac  = lipgloss.Color("#a78bfa")
	cLav    = lipgloss.Color("#c4b5fd")
	cPink   = lipgloss.Color("#ff7ac6")
	cGreen  = lipgloss.Color("#7ee787")
	cRed    = lipgloss.Color("#ff8b8b")
	cYellow = lipgloss.Color("#f2cc60")
	cBlue   = lipgloss.Color("#79c0ff")
	cText   = lipgloss.Color("#e6e6ef")
	cDim    = lipgloss.Color("#9a9ab8")
	cFaint  = lipgloss.Color("#5b5b73") // stands for the mockup's opacity .4
	cFaintB = lipgloss.Color("#2d2d40")
	cSelBg  = lipgloss.Color("#262040")
	cPage   = lipgloss.Color("#16161e") // page background
	cCard   = lipgloss.Color("#1c1c28") // card / modal background

	sText   = lipgloss.NewStyle().Foreground(cText)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sFaint  = lipgloss.NewStyle().Foreground(cFaint)
	sGreen  = lipgloss.NewStyle().Foreground(cGreen)
	sRed    = lipgloss.NewStyle().Foreground(cRed)
	sYellow = lipgloss.NewStyle().Foreground(cYellow)
	sPink   = lipgloss.NewStyle().Foreground(cPink)
	sLav    = lipgloss.NewStyle().Foreground(cLav)
	sBold   = lipgloss.NewStyle().Bold(true)
	sBadge  = lipgloss.NewStyle().Background(cViolet).Foreground(lipgloss.Color("#ffffff")).Bold(true).Padding(0, 1)
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spin(frame int) string { return spinFrames[frame%len(spinFrames)] }

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if d := w - lipgloss.Width(s); d > 0 {
		s += strings.Repeat(" ", d)
	}
	return s
}

// row lays out left and right text on one line of width w.
func row(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		if rw := lipgloss.Width(right); w-rw-1 > 2 {
			return fit(left, w-rw-1) + " " + right
		}
		return fit(left, w)
	}
	return left + strings.Repeat(" ", gap) + right
}

func hint(key, label string) string {
	return sBold.Foreground(cText).Render(key) + " " + sDim.Render(label)
}

func hints(h ...string) string { return strings.Join(h, "   ") }

// footer draws the rule and one line of text under a view.
func footer(w int, line string) string {
	return lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", w)) + "\n" + ansi.Truncate(line, w, "")
}

func header(left, right string, w int) string { return row(left, right, w) }

func fmtDur(secs int) string {
	if secs < 0 {
		secs = 0
	}
	if secs < 60 {
		return itoa(secs) + " s"
	}
	return itoa(secs/60) + " min " + itoa(secs%60) + " s"
}

func clock(secs int) string {
	if secs < 0 {
		secs = 0
	}
	return pad2(secs/60) + ":" + pad2(secs%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string { return strconv.Itoa(n) }

// plainDim strips styling from a rendered view and repaints it faint.
func plainDim(view string) []string {
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		lines[i] = sFaint.Render(ansi.Strip(l))
	}
	return lines
}

// overlay draws fg over bg with its top-left corner at (x, y).
func overlay(bg, fg []string, x, y int) []string {
	out := append([]string(nil), bg...)
	for i, l := range fg {
		r := y + i
		if r < 0 || r >= len(out) {
			continue
		}
		left := ansi.Truncate(out[r], x, "")
		if d := x - lipgloss.Width(left); d > 0 {
			left += strings.Repeat(" ", d)
		}
		right := ansi.TruncateLeft(out[r], x+lipgloss.Width(l), "")
		out[r] = left + "\x1b[0m" + l + "\x1b[0m" + right
	}
	return out
}

// bgSeq / fgSeq return the SGR sequence lipgloss would emit for a color on the
// current terminal profile ("" when colors are off).
func bgSeq(c lipgloss.Color) string { return sgr(lipgloss.NewStyle().Background(c)) }
func fgSeq(c lipgloss.Color) string { return sgr(lipgloss.NewStyle().Foreground(c)) }

func sgr(st lipgloss.Style) string {
	out := st.Render("x")
	if i := strings.Index(out, "x"); i > 0 {
		return out[:i]
	}
	return ""
}

// keepBg re-applies a background after every reset inside s, so that
// segments styled separately do not punch holes in it.
func keepBg(s string, bg lipgloss.Color) string {
	seq := bgSeq(bg)
	if seq == "" {
		return s
	}
	return strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+seq)
}

// paintPage fills the whole w×h screen with the page background and default
// text color.
func paintPage(view string, w, h int) string {
	bg, fg := bgSeq(cPage), fgSeq(cText)
	if bg == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	pre := bg + fg
	for i, l := range lines {
		if d := w - lipgloss.Width(l); d > 0 {
			l += strings.Repeat(" ", d)
		}
		lines[i] = pre + strings.ReplaceAll(l, "\x1b[0m", "\x1b[0m"+pre) + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}
