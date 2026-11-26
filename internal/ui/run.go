package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"todo/internal/board"
	"todo/internal/i18n"
	"todo/internal/runner"
)

const (
	stPending = "pending"
	stRunning = "running"
	stDone    = "done"
	stFailed  = "failed"
)

type stepInfo struct {
	name, status string
	start        time.Time
	dur          time.Duration
}

const (
	logInfo = iota
	logOut
	logErr
	logWarn
)

type logLine struct {
	t    time.Time
	kind int
	text string
}

// runState is the in-memory state of a workflow run attached to a card.
type runState struct {
	run        *runner.Run
	starting   bool
	status     string // running|completed|failed|cancelled
	paused     bool
	startedAt  time.Time
	endedAt    time.Time
	lastOutput time.Time
	steps      []stepInfo
	logs       []logLine
	ask, err   string
	follow     bool
	inputs     map[string]string
}

func newRunState(steps []string, inputs map[string]string) *runState {
	rs := &runState{starting: true, status: board.StatusRunning, startedAt: time.Now(), lastOutput: time.Now(), follow: true, inputs: inputs}
	for _, s := range steps {
		rs.steps = append(rs.steps, stepInfo{name: s, status: stPending})
	}
	return rs
}

func (rs *runState) active() bool { return rs.starting || rs.status == board.StatusRunning }

func (rs *runState) elapsed(now time.Time) time.Duration {
	if !rs.endedAt.IsZero() {
		return rs.endedAt.Sub(rs.startedAt)
	}
	return now.Sub(rs.startedAt)
}

func (rs *runState) step(name string) *stepInfo {
	for i := range rs.steps {
		if rs.steps[i].name == name {
			return &rs.steps[i]
		}
	}
	rs.steps = append(rs.steps, stepInfo{name: name, status: stPending})
	return &rs.steps[len(rs.steps)-1]
}

func (rs *runState) log(now time.Time, kind int, text string) {
	rs.logs = append(rs.logs, logLine{t: now, kind: kind, text: text})
}

// apply folds an event into the state and reports whether the run ended.
func (rs *runState) apply(ev runner.Event, now time.Time) bool {
	switch ev.Kind {
	case runner.KindStepStarted:
		s := rs.step(ev.Step)
		s.status, s.start = stRunning, now
		rs.log(now, logInfo, i18n.T("run.step", ev.Step))
		rs.lastOutput = now
	case runner.KindStepDone:
		s := rs.step(ev.Step)
		if !s.start.IsZero() {
			s.dur = now.Sub(s.start)
		}
		kind := logInfo
		s.status = stDone
		if ev.Status != "completed" {
			s.status, kind = stFailed, logWarn
		}
		rs.log(now, kind, fmt.Sprintf("%s ▸ %s", ev.Step, ev.Status))
		rs.lastOutput = now
	case runner.KindOutput:
		kind := logOut
		switch {
		case ev.Stream == "err":
			kind = logErr
		case strings.Contains(strings.ToLower(ev.Line), "warn"):
			kind = logWarn
		}
		rs.log(now, kind, ev.Line)
		rs.lastOutput = now
	case runner.KindAsk:
		rs.ask = ev.Prompt
	case runner.KindDone:
		rs.status, rs.ask, rs.err, rs.paused = ev.Status, "", ev.Error, false
		rs.endedAt = now
		for i := range rs.steps {
			if rs.steps[i].status == stRunning {
				rs.steps[i].dur = now.Sub(rs.steps[i].start)
				rs.steps[i].status = stFailed
				if ev.Status == board.StatusCompleted {
					rs.steps[i].status = stDone
				}
			}
		}
		if ev.Error != "" {
			rs.log(now, logErr, ev.Error)
		}
		return true
	}
	return false
}

// progress returns the 1-based current step, the step count and a percentage.
func (rs *runState) progress() (k, n, pct int) {
	n = len(rs.steps)
	if n == 0 {
		return 0, 0, 0
	}
	done := 0
	k = 0
	for i, s := range rs.steps {
		switch s.status {
		case stDone:
			done++
			if k == 0 {
				k = i + 1
			}
		case stRunning:
			k = i + 1
		}
	}
	if k == 0 {
		k = 1
	}
	if rs.status == board.StatusCompleted {
		return n, n, 100
	}
	return k, n, done * 100 / n
}

// ---- model glue ----

func (m *Model) runWidths() (left, right int) {
	left = 34
	if m.w < 100 {
		left = 26
	}
	right = m.w - left - 1
	if right < 20 {
		right = 20
	}
	return
}

func (m *Model) openRun(id string) {
	m.runCard = id
	m.mode = modeRun
	m.resize()
	m.refreshRun()
	if rs := m.runs[id]; rs != nil && rs.follow {
		m.vp.GotoBottom()
	}
}

func (m *Model) runContent() string {
	rs := m.runs[m.runCard]
	if rs == nil {
		return ""
	}
	w := m.vp.Width
	if w < 10 {
		w = 10
	}
	var out []string
	for _, l := range rs.logs {
		ts := sFaint.Render(l.t.Format("15:04:05")) + " "
		text := l.text
		var st lipgloss.Style
		switch l.kind {
		case logInfo:
			st = sDim
		case logWarn:
			st = sYellow
		case logErr:
			st = sRed
		default:
			st = sText
		}
		for i, seg := range strings.Split(ansi.Wrap(text, w-9, ""), "\n") {
			p := "         "
			if i == 0 {
				p = ts
			}
			out = append(out, p+st.Render(seg))
		}
	}
	if rs.active() && !rs.paused && rs.ask == "" && time.Since(rs.lastOutput) > 2*time.Second {
		out = append(out, sFaint.Render(time.Now().Format("15:04:05"))+" "+sPink.Render(spin(m.frame)+" "+i18n.T("run.waiting")))
	}
	return strings.Join(out, "\n")
}

func (m *Model) refreshRun() {
	if m.mode != modeRun {
		return
	}
	rs := m.runs[m.runCard]
	m.vp.SetContent(m.runContent())
	if rs != nil && rs.follow {
		m.vp.GotoBottom()
	}
}

func (m *Model) updateRun(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rs := m.runs[m.runCard]
	if rs != nil && rs.ask != "" && rs.run != nil {
		switch msg.String() {
		case "esc":
		case "ctrl+d":
			_ = rs.run.Answer(runner.EOF)
			rs.ask = ""
			return m, nil
		case "enter":
			txt := m.answer.Value()
			rs.log(time.Now(), logInfo, "› "+txt)
			_ = rs.run.Answer(txt)
			rs.ask = ""
			m.answer.SetValue("")
			m.refreshRun()
			return m, nil
		default:
			m.answer.Focus()
			var cmd tea.Cmd
			m.answer, cmd = m.answer.Update(msg)
			return m, cmd
		}
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeBoard
	case "c":
		if rs != nil && rs.run != nil && rs.active() {
			if rs.paused {
				_ = rs.run.Resume()
				rs.paused = false
			}
			_ = rs.run.Cancel()
		}
	case "p":
		if rs != nil && rs.run != nil && rs.active() {
			var err error
			if rs.paused {
				err = rs.run.Resume()
			} else {
				err = rs.run.Pause()
			}
			if err == nil {
				rs.paused = !rs.paused
			}
		}
	case "r":
		if rs != nil && !rs.active() {
			id := m.runCard
			delete(m.runs, id)
			cmd := m.startRun(id)
			m.refreshRun()
			return m, cmd
		}
	case "v":
		return m, m.openReview(m.runCard)
	case "f":
		if rs != nil {
			rs.follow = !rs.follow
			if rs.follow {
				m.vp.GotoBottom()
			}
		}
	default:
		if rs != nil {
			switch msg.String() {
			case "up", "down", "pgup", "pgdown", "home":
				rs.follow = false
			case "end":
				rs.follow = true
			}
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) viewRun() string {
	c := m.card(m.runCard)
	title, wf := "?", ""
	if c != nil {
		title, wf = c.Title, c.Workflow
	}
	rs := m.runs[m.runCard]
	if rs == nil {
		return sDim.Render(i18n.T("run.none"))
	}
	secs := int(rs.elapsed(time.Now()).Seconds())
	var status string
	switch {
	case rs.starting:
		status = sPink.Render(spin(m.frame) + " " + i18n.T("run.starting"))
	case rs.paused:
		status = sYellow.Render(i18n.T("run.paused", clock(secs)))
	case rs.status == board.StatusRunning:
		status = sPink.Render(spin(m.frame) + " " + i18n.T("run.running", clock(secs)))
	case rs.status == board.StatusCompleted:
		status = sGreen.Render(i18n.T("run.done", clock(secs)))
	case rs.status == board.StatusCancelled:
		status = sDim.Render(i18n.T("run.cancelled", clock(secs)))
	default:
		status = sRed.Render(i18n.T("run.failed", clock(secs)))
	}
	head := header(sBadge.Render("TODO")+"  "+sText.Render(title)+"  "+sDim.Render("/ "+wf), status, m.w)

	lw, rw := m.runWidths()
	ph := m.h - 3
	left := m.stepsPane(rs, lw, ph)
	right := paneBox(rw, ph, sBold.Foreground(cLav).Render(i18n.T("run.logs"))+"\n"+m.vp.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	var foot string
	if rs.ask != "" {
		foot = sPink.Render(rs.ask) + " " + m.answer.View()
	} else {
		follow := hint("f", i18n.T("h.follow"))
		if rs.follow {
			follow += sPink.Render(" ●")
		}
		foot = hints(hint("esc", i18n.T("h.back")), hint("p", map[bool]string{true: i18n.T("h.resume"), false: i18n.T("h.pause")}[rs.paused]),
			hint("c", i18n.T("h.cancel")), hint("↑↓", i18n.T("h.scroll")), follow, hint("r", i18n.T("h.retry")))
	}
	return head + "\n" + body + "\n" + footer(m.w, foot)
}

func paneBox(w, h int, content string) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cBorder).
		Padding(0, 1).Width(w - 2).Height(h - 2).Render(content)
}

func (m *Model) stepsPane(rs *runState, w, h int) string {
	inner := w - 4
	lines := []string{sBold.Foreground(cLav).Render(i18n.T("run.steps"))}
	now := time.Now()
	for _, s := range rs.steps {
		var icon, dur string
		st := sText
		switch s.status {
		case stDone:
			icon, st = sGreen.Render("✓"), sText
			dur = fmt.Sprintf("%d s", int(s.dur.Seconds()))
		case stFailed:
			icon, st = sRed.Render("✗"), sRed
			dur = fmt.Sprintf("%d s", int(s.dur.Seconds()))
		case stRunning:
			icon, st = sPink.Render(spin(m.frame)), sBold.Foreground(cPink)
			dur = fmt.Sprintf("%d s", int(now.Sub(s.start).Seconds()))
		default:
			icon, st = sDim.Render("○"), sDim
		}
		name := fit(s.name, inner-2-lipgloss.Width(dur)-1)
		lines = append(lines, row(icon+" "+st.Render(strings.TrimRight(name, " ")), sDim.Render(dur), inner))
	}
	if len(rs.steps) == 0 {
		lines = append(lines, sDim.Render(i18n.T("run.nosteps")))
	}
	// Inputs at the bottom.
	var in []string
	keys := make([]string, 0, len(rs.inputs))
	for k := range rs.inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		in = append(in, k+"="+rs.inputs[k])
	}
	bottom := []string{lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", inner)), sDim.Render(i18n.T("run.inputs"))}
	if len(in) == 0 {
		bottom = append(bottom, sDim.Render("—"))
	} else {
		bottom = append(bottom, strings.Split(ansi.Wrap(sText.Render(strings.Join(in, " · ")), inner, ""), "\n")...)
	}
	free := h - 2 - len(lines) - len(bottom)
	if free < 0 {
		free = 0
	}
	all := append(lines, make([]string, free)...)
	all = append(all, bottom...)
	return paneBox(w, h, strings.Join(all, "\n"))
}
