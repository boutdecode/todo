package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"todo/internal/board"
	"todo/internal/i18n"
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
			l += i18n.T("card.step", k, n)
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
		lines = append(lines, paint(sRed).Render(fit(i18n.T("card.failed", wf), tw)), paint(sDim).Render(fit(i18n.T("card.retry"), tw)))
	case status == board.StatusCancelled:
		lines = append(lines, paint(sDim).Render(fit(i18n.T("card.cancelled", wf), tw)), paint(sDim).Render(fit(i18n.T("card.retry"), tw)))
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
			lines = append(lines, paint(sDim).Render(fit(i18n.T("card.enter"), tw)))
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
		content := strings.Join(m.cardLines(c, sel, dimmed, tw), "\n")
		switch {
		case sel:
			st = st.Background(cSelBg).BorderBackground(cSelBg)
			content = keepBg(content, cSelBg)
		case !m.transparent:
			st = st.Background(cCard)
			content = keepBg(content, cCard)
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
	active := m.mode == modeFilter || m.filter != ""
	in := m.filter
	var right string
	switch {
	case m.mode == modeFilter:
		in = m.filterIn.View()
	case m.filter == "":
		in = sDim.Render(i18n.T("search.placeholder"))
	}
	if active {
		right = sDim.Render(i18n.N("search.found", total))
	} else {
		right = sDim.Render(i18n.N("search.idle", total))
	}
	border := cBorder
	if active {
		border = cLilac
	}
	left := sLilac().Render("/") + " " + in
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).
		Width(m.w - 2).Render(row(left, right, m.w-4))
}

func sLilac() lipgloss.Style { return sBold.Foreground(cLilac) }

// tabBar lists the todos; the active one is highlighted, a spinner marks running ones.
func (m *Model) tabBar() string {
	var parts []string
	for i, b := range m.boards {
		name := b.Title()
		if m.boardRuns(i) > 0 {
			name = spin(m.frame) + " " + name
		}
		name = " " + name + " "
		switch {
		case i == m.active:
			parts = append(parts, lipgloss.NewStyle().Background(cSelBg).Foreground(cLilac).Bold(true).Render(name))
		case m.boardRuns(i) > 0:
			parts = append(parts, sPink.Render(name))
		default:
			parts = append(parts, sDim.Render(name))
		}
	}
	return strings.Join(parts, " ")
}

func (m *Model) viewBoard() string {
	n := len(m.board.Columns)
	right := sDim.Render(i18n.T("head.project", projectName(m.dir)) + " · " + i18n.N("head.active", m.activeRuns()))
	if m.version != "" {
		right += sDim.Render(" · awf v" + m.version)
	}
	head := header(sBadge.Render("TODO")+"  "+m.tabBar(), right, m.w)

	areaH := m.h - 1 - 2
	bar := m.searchBar()
	parts := []string{head, bar}
	areaH -= lipgloss.Height(bar)
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
		foot = sRed.Render(map[bool]string{true: i18n.T("confirm.board", m.board.Title()), false: i18n.T("confirm.card")}[m.confirmBoard]) + hint("y", i18n.T("confirm.yes")) + "   " + hint(i18n.T("confirm.other"), i18n.T("confirm.no"))
	case m.mode == modeName:
		label := i18n.T("name.new")
		if m.renaming {
			label = i18n.T("name.rename")
		}
		foot = sLilac().Render(label) + m.nameIn.View() + "   " + hint("↵", i18n.T("h.confirm")) + "   " + hint("esc", i18n.T("h.cancel"))
	case m.msg != "":
		foot = sYellow.Render(m.msg)
	default:
		foot = hints(hint("←↑↓→", i18n.T("h.navigate")), hint("↵", i18n.T("h.run")), hint("tab", i18n.T("h.nexttodo")), hint("/", i18n.T("h.search")), hint("n", i18n.T("h.newtask")),
			hint("H L", i18n.T("h.move")), hint("l", i18n.T("h.logs")), hint("?", i18n.T("h.help")), hint("q", i18n.T("h.quit")))
	}
	parts = append(parts, footer(m.w, ansi.Truncate(foot, m.w, "")))
	return strings.Join(parts, "\n")
}
