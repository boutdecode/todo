package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"todo/internal/i18n"
	"todo/internal/review"
)

// reviewMsg carries the result of the asynchronous diff computation.
type reviewMsg struct {
	card string
	res  *review.Result
	err  error
}

// reviewState is the state of the code review view.
type reviewState struct {
	card    string
	from    mode // view to go back to
	loading bool
	res     *review.Result
	err     error
	sel     int // selected file
}

func reviewCmd(card, dir, base string) tea.Cmd {
	return func() tea.Msg {
		res, err := review.Diff(dir, base)
		return reviewMsg{card: card, res: res, err: err}
	}
}

// openReview shows what the last run of card id changed: its BaseRev against the
// working tree.
func (m *Model) openReview(id string) tea.Cmd {
	c := m.card(id)
	if c == nil {
		return nil
	}
	base := ""
	if c.LastRun != nil {
		base = c.LastRun.BaseRev
	}
	from := m.mode
	if from != modeRun {
		from = modeBoard
	}
	m.rv = reviewState{card: id, from: from, loading: true}
	m.mode = modeReview
	m.resize()
	m.rvp.SetContent("")
	return reviewCmd(id, m.dir, base)
}

func (m *Model) onReview(msg reviewMsg) {
	if m.mode != modeReview || msg.card != m.rv.card || !m.rv.loading {
		return // stale result: the view was closed or reopened meanwhile
	}
	m.rv.loading, m.rv.res, m.rv.err, m.rv.sel = false, msg.res, msg.err, 0
	m.rvp.Height = max(m.h-3-3, 1)
	if msg.res != nil && msg.res.Fallback {
		m.rvp.Height = max(m.rvp.Height-1, 1) // room for the fallback note
	}
	m.showReviewFile()
}

func (m *Model) reviewFiles() []review.File {
	if m.rv.res == nil {
		return nil
	}
	return m.rv.res.Files
}

func (m *Model) showReviewFile() {
	m.rvp.SetContent(m.reviewContent())
	m.rvp.GotoTop()
}

func (m *Model) reviewContent() string {
	files := m.reviewFiles()
	if m.rv.sel >= len(files) {
		return ""
	}
	f := files[m.rv.sel]
	w := max(m.rvp.Width, 10)
	if f.Binary {
		return sDim.Render(i18n.T("review.binary"))
	}
	const num = 5 // line number column width
	var out []string
	for _, l := range f.Lines {
		if l.Kind == review.Hunk {
			out = append(out, sLav.Render(ansi.Truncate(l.Text, w, "…")))
			continue
		}
		if l.Kind == review.Note {
			out = append(out, sFaint.Render(strings.Repeat(" ", 2*num+2)+l.Text))
			continue
		}
		n, sign, st := l.New, " ", sText
		switch l.Kind {
		case review.Add:
			sign, st = "+", sGreen
		case review.Del:
			n, sign, st = l.Old, "-", sRed
		}
		gutter := sFaint.Render(fmt.Sprintf("%*d ", num, n))
		text := strings.ReplaceAll(l.Text, "\t", "    ")
		for i, seg := range strings.Split(ansi.Wrap(text, w-num-3, ""), "\n") {
			g, sg := strings.Repeat(" ", num+1), " "
			if i == 0 {
				g, sg = gutter, sign
			}
			out = append(out, g+st.Render(sg+" "+seg))
		}
	}
	if f.Truncated {
		out = append(out, sYellow.Render(i18n.T("review.truncated")))
	}
	if len(out) == 0 {
		return sDim.Render(i18n.T("review.nochange"))
	}
	return strings.Join(out, "\n")
}

func (m *Model) selectReviewFile(d int) {
	n := len(m.reviewFiles())
	if n == 0 {
		return
	}
	m.rv.sel = ((m.rv.sel+d)%n + n) % n
	m.showReviewFile()
}

func (m *Model) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "v":
		m.mode = m.rv.from
		m.rv.loading = false
		if m.mode == modeRun {
			m.refreshRun()
		}
	case "tab", "n", "j":
		m.selectReviewFile(1)
	case "shift+tab", "p", "k":
		m.selectReviewFile(-1)
	case "r":
		if !m.rv.loading {
			return m, m.reloadReview()
		}
	default:
		var cmd tea.Cmd
		m.rvp, cmd = m.rvp.Update(msg)
		return m, cmd
	}
	return m, nil
}

// reloadReview recomputes the diff of the card being reviewed.
func (m *Model) reloadReview() tea.Cmd {
	from := m.rv.from
	cmd := m.openReview(m.rv.card)
	m.rv.from = from
	return cmd
}

func (m *Model) viewReview() string {
	c := m.card(m.rv.card)
	title := "?"
	if c != nil {
		title = c.Title
	}
	var status string
	switch {
	case m.rv.loading:
		status = sPink.Render(spin(m.frame) + " " + i18n.T("review.loading"))
	case m.rv.err != nil:
		status = sRed.Render(firstLine(m.rv.err.Error()))
	case m.rv.res != nil:
		adds, dels := 0, 0
		for _, f := range m.rv.res.Files {
			adds += f.Adds
			dels += f.Dels
		}
		status = sDim.Render(i18n.N("review.files", len(m.rv.res.Files))+" · ") + sGreen.Render(fmt.Sprintf("+%d", adds)) + " " + sRed.Render(fmt.Sprintf("-%d", dels))
	}
	head := header(sBadge.Render("TODO")+"  "+sText.Render(title)+"  "+sDim.Render("/ "+i18n.T("review.title")), status, m.w)

	lw, rw := m.runWidths()
	ph := m.h - 3
	left := paneBox(lw, ph, m.reviewList(lw-4, ph-2))
	rtitle := sBold.Foreground(cLav).Render(i18n.T("review.diff"))
	if files := m.reviewFiles(); m.rv.sel < len(files) {
		rtitle += " " + sDim.Render(files[m.rv.sel].Path)
	}
	content := ansi.Truncate(rtitle, rw-4, "…") + "\n"
	if m.rv.res != nil && m.rv.res.Fallback {
		content += ansi.Truncate(sYellow.Render(i18n.T("review.fallback")), rw-4, "…") + "\n"
	}
	right := paneBox(rw, ph, content+m.rvp.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	foot := hints(hint("esc", i18n.T("h.back")), hint("tab", i18n.T("h.nextfile")), hint("↑↓", i18n.T("h.scroll")), hint("r", i18n.T("h.reload")))
	return head + "\n" + body + "\n" + footer(m.w, foot)
}

func (m *Model) reviewList(w, h int) string {
	files := m.reviewFiles()
	lines := []string{sBold.Foreground(cLav).Render(i18n.T("review.changes"))}
	switch {
	case m.rv.loading:
		lines = append(lines, sDim.Render(i18n.T("review.loading")))
	case m.rv.err != nil:
		lines = append(lines, sDim.Render("—"))
	case len(files) == 0:
		lines = append(lines, sDim.Render(i18n.T("review.nochange")))
	}
	// keep the selection visible
	avail := max(h-len(lines), 1)
	start := 0
	if m.rv.sel >= avail {
		start = m.rv.sel - avail + 1
	}
	for i := start; i < len(files) && i < start+avail; i++ {
		f := files[i]
		st := sGreen
		switch f.Status() {
		case "D":
			st = sRed
		case "M", "R":
			st = sYellow
		}
		stat := sGreen.Render(fmt.Sprintf("+%d", f.Adds)) + sRed.Render(fmt.Sprintf(" -%d", f.Dels))
		name := f.Path
		if room := max(w-2-lipgloss.Width(stat)-1, 1); lipgloss.Width(name) > room {
			name = ansi.TruncateLeft(name, lipgloss.Width(name)-room+1, "…")
		}
		label := st.Render(f.Status()) + " " + sText.Render(name)
		if i == m.rv.sel {
			label = st.Render(f.Status()) + " " + sBold.Foreground(cPink).Render(name)
		}
		lines = append(lines, row(label, stat, w))
	}
	return strings.Join(lines, "\n")
}
