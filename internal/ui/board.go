package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"todo/internal/board"
)

// cardLines builds the text lines of a card (without the frame).
func (m *Model) cardLines(c board.Card, selected, dimmed bool, tw int) []string {
	paint := func(st lipgloss.Style) lipgloss.Style {
		if dimmed {
			return sFaint
		}
		return st
	}
	title := c.Title
	tst := paint(sText)
	if selected {
		title, tst = "▸ "+title, paint(sBold.Foreground(cText))
	}
	lines := []string{tst.Render(fit(title, tw))}

	rs := m.runs[c.ID]
	status, secs := "", 0
	if rs != nil {
		status, secs = rs.status, int(rs.elapsed(time.Now()).Seconds())
	} else if c.LastRun != nil {
		status, secs = c.LastRun.Status, c.LastRun.Secs
		if status == board.StatusRunning { // left over from a previous session
			status = board.StatusCancelled
		}
	}
	wf := c.Workflow
	switch {
	case rs != nil && rs.active():
		k, n, pct := rs.progress()
		l := spin(m.frame) + " " + wf
		if rs.paused {
			l = "⏸ " + wf
		}
		if n > 0 {
			l += " · étape " + itoa(k) + "/" + itoa(n)
		}
		lines = append(lines, paint(sPink).Render(fit(l, tw)))
		if n > 0 {
			const bar = 11
			filled := pct * bar / 100
			lines = append(lines, paint(sPink).Render(fit(strings.Repeat("█", filled)+strings.Repeat("░", bar-filled)+" "+itoa(pct)+"%", tw)))
		}
	case status == board.StatusCompleted:
		lines = append(lines, paint(sGreen).Render(fit("✓ "+wf+" · "+fmtDur(secs), tw)))
	case status == board.StatusFailed:
		lines = append(lines, paint(sRed).Render(fit("✗ "+wf+" · échec", tw)), paint(sDim).Render(fit("r pour relancer", tw)))
	case status == board.StatusCancelled:
		lines = append(lines, paint(sDim).Render(fit("⊘ "+wf+" · annulé", tw)), paint(sDim).Render(fit("r pour relancer", tw)))
	default:
		l := "wf: —"
		if wf != "" {
			l = "wf: " + wf
		}
		st := sDim
		if selected && wf != "" {
			st = sLav
		}
		lines = append(lines, paint(st).Render(fit(l, tw)))
		if selected && wf != "" {
			lines = append(lines, paint(sDim).Render(fit("entrée ↵ pour lancer", tw)))
		}
	}
	return lines
}

func (m *Model) renderColumn(ci int, col board.Column, colW, areaH int) string {
	focused := ci == m.col
	border := cBorder
	if col.AutoRun {
		border = cViolet
	} else if focused && len(col.Cards) == 0 {
		border = cLilac
	}
	inner := colW - 4
	tc := cText
	if col.Color != "" {
		tc = lipgloss.Color(col.Color)
	}
	dimCol := m.filter != "" && len(m.vis(ci)) == 0
	titleSt := sBold.Foreground(tc)
	if dimCol {
		titleSt = sFaint
	}
	head := row(titleSt.Render(col.Name), sDim.Render(itoa(len(col.Cards))), inner)

	matches := m.vis(ci)
	selIdx := -1
	if focused && m.sel[ci] < len(matches) {
		selIdx = matches[m.sel[ci]]
	}
	// Render every card, then keep a window that contains the selection.
	tw := inner - 4
	boxes := make([]string, len(col.Cards))
	for i, c := range col.Cards {
		dimmed := m.filter != "" && !m.match(c)
		sel := i == selIdx
		bc := cBorder
		switch {
		case sel:
			bc = cLilac
		case dimmed:
			bc = cFaintB
		}
		st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(bc).Padding(0, 1).Width(inner - 2)
		if sel {
			st = st.Background(cSelBg)
		}
		content := strings.Join(m.cardLines(c, sel, dimmed, tw), "\n")
		if sel { // paint the background under styled segments
			content = strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m\x1b[48;2;38;32;64m")
		}
		boxes[i] = st.Render(content)
	}
	avail := areaH - 2 - 1
	start := 0
	if selIdx >= 0 {
		for start < selIdx && heightSum(boxes[start:selIdx+1]) > avail {
			start++
		}
	}
	lines := []string{head}
	used := 0
	for i := start; i < len(boxes); i++ {
		h := lipgloss.Height(boxes[i])
		if used+h > avail {
			break
		}
		lines = append(lines, boxes[i])
		used += h
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).
		Width(colW - 2).Height(areaH - 2).Render(strings.Join(lines, "\n"))
}

func heightSum(bs []string) int {
	n := 0
	for _, b := range bs {
		n += lipgloss.Height(b)
	}
	return n
}

func (m *Model) searchBar() string {
	total := 0
	for ci := range m.board.Columns {
		total += len(m.vis(ci))
	}
	word := "résultats"
	if total <= 1 {
		word = "résultat"
	}
	in := m.filter
	if m.mode == modeFilter {
		in = m.filterIn.View()
	}
	left := sLilac().Render("/") + " " + in
	right := sDim.Render(itoa(total) + " " + word + " · titre, workflow · esc effacer")
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cLilac).Padding(0, 1).
		Width(m.w - 2).Render(row(left, right, m.w-4))
}

func sLilac() lipgloss.Style { return sBold.Foreground(cLilac) }

func (m *Model) viewBoard() string {
	n := len(m.board.Columns)
	right := sDim.Render(itoa(m.activeRuns()) + " workflows actifs")
	if m.version != "" {
		right += sDim.Render(" · awf v" + m.version)
	}
	head := header(sBadge.Render("TODO")+"  "+sDim.Render("projet: "+projectName(m.dir)), right, m.w)

	searching := m.mode == modeFilter || m.filter != ""
	areaH := m.h - 1 - 2
	parts := []string{head}
	if searching {
		bar := m.searchBar()
		parts = append(parts, bar)
		areaH -= lipgloss.Height(bar)
	}
	if areaH < 5 {
		areaH = 5
	}
	colW := (m.w - (n - 1)) / n
	cols := make([]string, 0, 2*n)
	for ci, col := range m.board.Columns {
		if ci > 0 {
			cols = append(cols, " ")
		}
		cols = append(cols, m.renderColumn(ci, col, colW, areaH))
	}
	parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, cols...))

	var foot string
	switch {
	case m.mode == modeConfirm:
		foot = sRed.Render("Supprimer cette carte ? ") + hint("y", "oui") + "   " + hint("autre", "non")
	case m.msg != "":
		foot = sYellow.Render(m.msg)
	default:
		foot = hints(hint("←↑↓→", "naviguer"), hint("↵", "lancer workflow"), hint("/", "rechercher"), hint("n", "nouvelle tâche"),
			hint("H L", "déplacer"), hint("l", "logs"), hint("?", "aide"), hint("q", "quitter"))
	}
	parts = append(parts, footer(m.w, ansi.Truncate(foot, m.w, "")))
	return strings.Join(parts, "\n")
}
