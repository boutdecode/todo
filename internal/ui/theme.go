// Package ui implements the Bubble Tea interface of todo.
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Same palette as awf-tui.
var (
	cBg     = lipgloss.Color("#17141f")
	cBorder = lipgloss.Color("#4a3f6b")
	cAccent = lipgloss.Color("#7d56f4")
	cPurple = lipgloss.Color("#a58bff")
	cPink   = lipgloss.Color("#ff6eb4")
	cOK     = lipgloss.Color("#5fd7a0")
	cBad    = lipgloss.Color("#ff7a8a")
	cRun    = lipgloss.Color("#f5d76e")
	cDim    = lipgloss.Color("#9a92b0")
	cText   = lipgloss.Color("#e6e1f0")
	cSelBg  = lipgloss.Color("#2a2440")

	sText   = lipgloss.NewStyle().Foreground(cText)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sBad    = lipgloss.NewStyle().Foreground(cBad)
	sRun    = lipgloss.NewStyle().Foreground(cRun)
	sPurple = lipgloss.NewStyle().Foreground(cPurple)
	sBold   = lipgloss.NewStyle().Bold(true)
	sLogo   = lipgloss.NewStyle().Background(cPink).Foreground(cBg).Bold(true).Padding(0, 1)
	sKey    = lipgloss.NewStyle().Background(cSelBg).Foreground(cText).Padding(0, 1)
)

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

// wrap hard-wraps s to width w and returns at most max lines.
func wrap(s string, w, max int) []string {
	if w <= 0 || max <= 0 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		w2 := ansi.Wrap(para, w, "")
		out = append(out, strings.Split(w2, "\n")...)
	}
	if len(out) > max {
		out = out[:max]
		out[max-1] = fit(out[max-1], w-1) + "…"
		out[max-1] = strings.TrimRight(out[max-1], " ")
	}
	return out
}

func hint(key, label string) string { return sKey.Render(key) + " " + sDim.Render(label) }
