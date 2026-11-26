package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"todo/internal/board"
	"todo/internal/config"
	"todo/internal/i18n"
	"todo/internal/notify"
	"todo/internal/runner"
)

type mode int

const (
	modeBoard mode = iota
	modeForm
	modeRun
	modeConfirm
	modeFilter
	modeHelp
	modeName // typing a todo name (create / rename)
)

type (
	runStartedMsg struct {
		card string
		run  *runner.Run
		err  error
	}
	runEventMsg struct {
		card string
		ev   runner.Event
		ch   <-chan runner.Event
	}
	tickMsg time.Time
)

// Notifier raises desktop notifications; *notify.Notifier implements it.
type Notifier interface {
	Show(key string, kind notify.Kind, summary, body string)
}

// Model is the root Bubble Tea model.
type Model struct {
	dir, bin string
	version  string
	cfg      *config.Config
	board    *board.Board // todo actif
	boards   []*board.Board
	active   int        // index of the active todo
	tabs     []tabState // remembered cursor per todo
	wfs      []runner.Workflow

	mode          mode
	col           int
	sel           []int // position among the matching cards, per column
	filter        string
	filterIn      textinput.Model
	w, h          int
	fullW, fullH  int
	msg           string
	runs          map[string]*runState
	form          *form
	runCard       string
	vp            viewport.Model
	answer        textinput.Model
	confirmCardID string
	confirmBoard  bool // the confirmation is about the whole todo
	nameIn        textinput.Model
	renaming      bool
	frame         int
	ticking       bool
	transparent   bool // keep the terminal background

	nt        Notifier // nil: no notifications
	focusSeen bool     // the terminal reports focus changes
	focused   bool     // the window has the focus (only meaningful once focusSeen)
}

// New builds the model for a project directory.
func New(dir, bin string, cfg *config.Config, boards []*board.Board) *Model {
	fi := textinput.New()
	fi.Prompt = ""
	ans := textinput.New()
	ans.Prompt = "› "
	ni := textinput.New()
	ni.Prompt = ""
	m := &Model{
		dir: dir, bin: bin, cfg: cfg, board: boards[0], boards: boards,
		wfs:  runner.ListWorkflows(dir),
		sel:  make([]int, len(boards[0].Columns)),
		runs: map[string]*runState{}, filterIn: fi, answer: ans, nameIn: ni,
		w: 100, h: 30,
	}
	m.tabs = make([]tabState, len(boards))
	m.resize()
	return m
}

// SetTransparent keeps the terminal's own background instead of painting the mockup's.
func (m *Model) SetTransparent(t bool) { m.transparent = t }

// SetNotifier enables notifications; they are only raised while the window is unfocused.
func (m *Model) SetNotifier(n Notifier) { m.nt = n }

// notify raises a notification for card id unless the window has the focus. Terminals
// that never report focus are treated as unfocused: better a spare notification than a
// missed question.
func (m *Model) notify(id string, kind notify.Kind, body string) {
	if m.nt == nil || m.focused {
		return
	}
	key := "notif.done"
	switch kind {
	case notify.Failure:
		key = "notif.failed"
	case notify.Ask:
		key = "notif.ask"
	}
	m.nt.Show(id, kind, i18n.T(key, projectName(m.dir)), body)
}

func (m *Model) notifyEnabled(kind notify.Kind) bool {
	if m.cfg == nil {
		return true
	}
	switch kind {
	case notify.Failure:
		return m.cfg.Notifications.Failure()
	case notify.Ask:
		return m.cfg.Notifications.Ask()
	}
	return m.cfg.Notifications.Success()
}

// cardTitle finds the title of card id in any todo (it may belong to a non-active one).
func (m *Model) cardTitle(id string) string {
	for _, b := range m.boards {
		if ci, idx, ok := b.Find(id); ok {
			return b.Columns[ci].Cards[idx].Title
		}
	}
	return ""
}

// notifyEnd reports the end of a run (cancellations are silent).
func (m *Model) notifyEnd(id, title, status, errText string, elapsed time.Duration) {
	switch status {
	case board.StatusCompleted:
		if m.notifyEnabled(notify.Success) {
			m.notify(id, notify.Success, fmt.Sprintf("%s · %s", title, elapsed.Round(time.Second)))
		}
	case board.StatusFailed:
		if m.notifyEnabled(notify.Failure) {
			body := title
			if e := truncate(firstLine(strings.TrimSpace(errText)), 120); e != "" {
				body += "\n" + e
			}
			m.notify(id, notify.Failure, body)
		}
	}
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// SetVersion records the awf version shown in the header.
func (m *Model) SetVersion(v string) { m.version = v }

func (m *Model) Init() tea.Cmd { return nil }

// ---- helpers ----

// tabState remembers the cursor of a todo while another one is shown.
type tabState struct {
	col int
	sel []int
}

// load makes todo i the active one, keeping the cursor of the previous one.
func (m *Model) load(i int) {
	m.tabs[m.active] = tabState{col: m.col, sel: m.sel}
	m.show(i)
}

// show displays todo i without remembering the previous cursor.
func (m *Model) show(i int) {
	m.active, m.board = i, m.boards[i]
	t := m.tabs[i]
	m.col, m.sel = t.col, t.sel
	if len(m.sel) != len(m.board.Columns) {
		m.sel = make([]int, len(m.board.Columns))
		m.col = 0
	}
	if m.col >= len(m.board.Columns) {
		m.col = 0
	}
	m.clamp()
}

// switchTab shows the todo d positions away (cyclic) and clears the search.
func (m *Model) switchTab(d int) {
	n := len(m.boards)
	if n < 2 {
		return
	}
	m.filter = ""
	m.filterIn.SetValue("")
	m.load(((m.active+d)%n + n) % n)
}

// boardRuns counts the running workflows of todo i.
func (m *Model) boardRuns(i int) int {
	n := 0
	for id, rs := range m.runs {
		if _, _, ok := m.boards[i].Find(id); ok && rs.active() {
			n++
		}
	}
	return n
}

// createBoard adds a todo named name and shows it.
func (m *Model) createBoard(name string) {
	b := board.New(m.dir, name, m.cfg.Columns)
	m.boards = append(m.boards, b)
	m.tabs = append(m.tabs, tabState{sel: make([]int, len(b.Columns))})
	m.load(len(m.boards) - 1)
	if err := b.Save(); err != nil {
		m.msg = i18n.T("err.save", err.Error())
	}
}

// deleteBoard removes the active todo (never the last one, never while it runs).
func (m *Model) deleteBoard() {
	if len(m.boards) < 2 {
		m.msg = i18n.T("err.lastboard")
		return
	}
	if m.boardRuns(m.active) > 0 {
		m.msg = i18n.T("err.boardruns")
		return
	}
	old := m.board
	i := m.active
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	m.boards = append(m.boards[:i], m.boards[i+1:]...)
	m.show(max(i-1, 0))
	if err := old.Delete(); err != nil {
		m.msg = i18n.T("err.delete", err.Error())
	}
}

func (m *Model) save() {
	if err := m.board.Save(); err != nil {
		m.msg = i18n.T("err.save", err.Error())
	}
}

func (m *Model) match(c board.Card) bool {
	if m.filter == "" {
		return true
	}
	f := strings.ToLower(m.filter)
	return strings.Contains(strings.ToLower(c.Title+" "+c.Workflow), f)
}

// vis returns the indices of the cards of column ci matching the filter.
func (m *Model) vis(ci int) []int {
	var out []int
	for i, c := range m.board.Columns[ci].Cards {
		if m.match(c) {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) clamp() {
	for ci := range m.board.Columns {
		n := len(m.vis(ci))
		if m.sel[ci] >= n {
			m.sel[ci] = n - 1
		}
		if m.sel[ci] < 0 {
			m.sel[ci] = 0
		}
	}
}

// cur returns the selected card's column and index.
func (m *Model) cur() (ci, idx int, ok bool) {
	v := m.vis(m.col)
	if m.sel[m.col] >= len(v) {
		return m.col, 0, false
	}
	return m.col, v[m.sel[m.col]], true
}

func (m *Model) curCard() *board.Card {
	if ci, idx, ok := m.cur(); ok {
		return &m.board.Columns[ci].Cards[idx]
	}
	return nil
}

// focus moves the cursor onto the card with id.
func (m *Model) focus(id string) {
	ci, idx, ok := m.board.Find(id)
	if !ok {
		return
	}
	m.col = ci
	for p, i := range m.vis(ci) {
		if i == idx {
			m.sel[ci] = p
		}
	}
}

func (m *Model) card(id string) *board.Card {
	ci, idx, ok := m.board.Find(id)
	if !ok {
		return nil
	}
	return &m.board.Columns[ci].Cards[idx]
}

func (m *Model) workflow(name string) runner.Workflow {
	for _, w := range m.wfs {
		if w.Name == name {
			return w
		}
	}
	return runner.Load(m.dir, name)
}

func (m *Model) activeRuns() int {
	n := 0
	for _, rs := range m.runs {
		if rs.active() {
			n++
		}
	}
	return n
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func projectName(dir string) string { return filepath.Base(dir) }

func shortDir(d string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(d, home) {
		return "~" + strings.TrimPrefix(d, home)
	}
	return d
}

// ---- runs ----

func startCmd(bin, dir string, c board.Card) tea.Cmd {
	return func() tea.Msg {
		r, err := runner.Start(bin, dir, c.Workflow, c.Inputs)
		return runStartedMsg{card: c.ID, run: r, err: err}
	}
}

func waitEvent(id string, ch <-chan runner.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			ev = runner.Event{Kind: runner.KindDone, Status: "failed", Error: i18n.T("err.stream")}
		}
		return runEventMsg{card: id, ev: ev, ch: ch}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// startRun starts the workflow of card id unless one is already running.
func (m *Model) startRun(id string) tea.Cmd {
	c := m.card(id)
	if c == nil {
		return nil
	}
	if c.Workflow == "" {
		m.msg = i18n.T("err.nowf")
		return nil
	}
	if rs := m.runs[id]; rs != nil && rs.active() {
		m.msg = i18n.T("err.running")
		return nil
	}
	m.runs[id] = newRunState(m.workflow(c.Workflow).Steps, c.Inputs)
	c.LastRun = &board.LastRun{Status: board.StatusRunning, At: time.Now()}
	m.save()
	m.msg = i18n.T("msg.starting", c.Workflow)
	cmds := []tea.Cmd{startCmd(m.bin, m.dir, *c)}
	if !m.ticking {
		m.ticking = true
		cmds = append(cmds, tickCmd())
	}
	return tea.Batch(cmds...)
}

// launch moves the card to the first execution column and runs it.
func (m *Model) launch(id string) tea.Cmd {
	c := m.card(id)
	if c == nil {
		return nil
	}
	if c.Workflow == "" {
		m.msg = i18n.T("err.nowf")
		return nil
	}
	if rs := m.runs[id]; rs != nil && rs.active() {
		m.msg = i18n.T("err.running")
		return nil
	}
	if rc := m.board.RunColumn(); rc >= 0 {
		if ci, idx, ok := m.board.Find(id); ok && ci != rc {
			m.board.Move(ci, idx, rc)
			m.clamp()
		}
	}
	m.focus(id)
	return m.startRun(id)
}

func (m *Model) onEvent(msg runEventMsg) tea.Cmd {
	rs := m.runs[msg.card]
	if rs == nil {
		return nil
	}
	prevAsk := rs.ask
	title := m.cardTitle(msg.card)
	if rs.apply(msg.ev, time.Now()) {
		m.notifyEnd(msg.card, title, msg.ev.Status, msg.ev.Error, rs.elapsed(time.Now()))
		m.finish(msg.card, msg.ev)
		m.refreshRun()
		return nil
	}
	if rs.ask != "" && prevAsk == "" && m.notifyEnabled(notify.Ask) {
		body := title
		if msg.ev.Step != "" {
			body += " · " + msg.ev.Step
		}
		m.notify(msg.card, notify.Ask, body)
	}
	m.refreshRun()
	return waitEvent(msg.card, msg.ch)
}

// finish records the result on the card and applies the column's OnSuccess/OnFailure.
func (m *Model) finish(id string, ev runner.Event) {
	if _, _, ok := m.board.Find(id); !ok {
		// the card lives in another todo: work on it, then come back
		back := m.active
		for i, b := range m.boards {
			if _, _, ok := b.Find(id); ok {
				m.load(i)
				defer m.load(back)
				break
			}
		}
	}
	ci, idx, ok := m.board.Find(id)
	if !ok {
		return
	}
	c := &m.board.Columns[ci].Cards[idx]
	rs := m.runs[id]
	runID, secs := "", 0
	if rs != nil {
		if rs.run != nil {
			runID = rs.run.ID
		}
		secs = int(rs.elapsed(time.Now()).Seconds())
	}
	c.LastRun = &board.LastRun{ID: runID, Status: ev.Status, At: time.Now(), Secs: secs}
	target := ""
	switch ev.Status {
	case board.StatusCompleted:
		target = m.board.Columns[ci].OnSuccess
	case board.StatusFailed:
		target = m.board.Columns[ci].OnFailure
	}
	m.msg = fmt.Sprintf("%s : %s", c.Title, ev.Status)
	hadFocus := false
	if cc := m.curCard(); cc != nil && cc.ID == id {
		hadFocus = true
	}
	if to := m.board.ColumnIndex(target); to >= 0 && to != ci {
		m.board.Move(ci, idx, to)
	}
	m.save()
	m.clamp()
	if hadFocus {
		m.focus(id) // the cursor follows the card
	}
}

// ---- update ----

func (m *Model) resize() {
	lw, rw := m.runWidths()
	_ = lw
	m.vp.Width = rw - 4
	m.syncRunLayout()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.fullW, m.fullH = msg.Width, msg.Height
		m.w, m.h = max(msg.Width-2*padX, 1), max(msg.Height-2*padY, 1)
		m.resize()
		m.refreshRun()
		return m, nil
	case tickMsg:
		m.frame++
		if m.activeRuns() == 0 && !m.anyStarting() {
			m.ticking = false
			return m, nil
		}
		m.refreshRun()
		return m, tickCmd()
	case runStartedMsg:
		rs := m.runs[msg.card]
		if rs == nil {
			return m, nil
		}
		rs.starting = false
		if msg.err != nil {
			ev := runner.Event{Kind: runner.KindDone, Status: "failed", Error: msg.err.Error()}
			rs.apply(ev, time.Now())
			m.notifyEnd(msg.card, m.cardTitle(msg.card), ev.Status, ev.Error, rs.elapsed(time.Now()))
			m.finish(msg.card, ev)
			m.msg = i18n.T("err.launch", firstLine(msg.err.Error()))
			m.refreshRun()
			return m, nil
		}
		rs.run = msg.run
		m.msg = i18n.T("msg.run", msg.run.ID)
		return m, waitEvent(msg.card, msg.run.Events)
	case runEventMsg:
		return m, m.onEvent(msg)
	case tea.FocusMsg:
		m.focusSeen, m.focused = true, true
		return m, nil
	case tea.BlurMsg:
		m.focusSeen, m.focused = true, false
		return m, nil
	case tea.KeyMsg:
		if m.focusSeen {
			m.focused = true // a key press proves the window has the focus
		}
		switch m.mode {
		case modeForm:
			return m.updateForm(msg)
		case modeRun:
			return m.updateRun(msg)
		case modeName:
			switch msg.String() {
			case "enter":
				if name := strings.TrimSpace(m.nameIn.Value()); name != "" {
					if m.renaming {
						m.board.Name = name
						m.save()
					} else {
						m.createBoard(name)
					}
				}
				m.mode = modeBoard
			case "esc":
				m.mode = modeBoard
			default:
				var cmd tea.Cmd
				m.nameIn, cmd = m.nameIn.Update(msg)
				return m, cmd
			}
			return m, nil
		case modeConfirm:
			if msg.String() == "y" && m.confirmBoard {
				m.deleteBoard()
			} else if msg.String() == "y" {
				if ci, idx, ok := m.board.Find(m.confirmCardID); ok {
					m.board.Remove(ci, idx)
					m.save()
					m.clamp()
				}
			}
			m.mode = modeBoard
			return m, nil
		case modeFilter:
			switch msg.String() {
			case "enter":
				m.mode = modeBoard
			case "esc":
				m.filter = ""
				m.filterIn.SetValue("")
				m.mode = modeBoard
			default:
				var cmd tea.Cmd
				m.filterIn, cmd = m.filterIn.Update(msg)
				m.filter = m.filterIn.Value()
				m.clamp()
				return m, cmd
			}
			m.clamp()
			return m, nil
		case modeHelp:
			m.mode = modeBoard
			return m, nil
		}
		return m.updateBoard(msg)
	}
	return m, nil
}

func (m *Model) anyStarting() bool {
	for _, rs := range m.runs {
		if rs.starting {
			return true
		}
	}
	return false
}

func (m *Model) updateBoard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.msg = ""
	nc := len(m.board.Columns)
	switch msg.String() {
	case "q", "ctrl+c":
		for _, rs := range m.runs {
			if rs.run != nil && rs.active() {
				_ = rs.run.Cancel()
			}
		}
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "tab":
		m.switchTab(1)
	case "shift+tab":
		m.switchTab(-1)
	case "N":
		m.renaming = false
		m.nameIn.SetValue("")
		m.mode = modeName
		return m, m.nameIn.Focus()
	case "R":
		m.renaming = true
		m.nameIn.SetValue(m.board.Title())
		m.mode = modeName
		return m, m.nameIn.Focus()
	case "D":
		m.confirmBoard = true
		m.mode = modeConfirm
	case "left":
		if m.col > 0 {
			m.col--
		}
	case "right":
		if m.col < nc-1 {
			m.col++
		}
	case "up":
		if m.sel[m.col] > 0 {
			m.sel[m.col]--
		}
	case "down":
		if m.sel[m.col] < len(m.vis(m.col))-1 {
			m.sel[m.col]++
		}
	case "H", "shift+left":
		return m, m.moveCard(-1)
	case "L", "shift+right":
		return m, m.moveCard(1)
	case "K", "shift+up":
		m.reorder(-1)
	case "J", "shift+down":
		m.reorder(1)
	case "n":
		m.form = newForm(m, nil)
		m.mode = modeForm
		return m, m.form.focusCmd()
	case "e":
		if c := m.curCard(); c != nil {
			m.form = newForm(m, c)
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
	case "d":
		if c := m.curCard(); c != nil {
			m.confirmCardID = c.ID
			m.confirmBoard = false
			m.mode = modeConfirm
		}
	case "enter":
		if c := m.curCard(); c != nil {
			if rs := m.runs[c.ID]; rs != nil && rs.active() {
				m.openRun(c.ID)
				break
			}
			return m, m.launch(c.ID)
		}
	case "r":
		if c := m.curCard(); c != nil {
			return m, m.launch(c.ID)
		}
	case "l":
		if c := m.curCard(); c != nil {
			if m.runs[c.ID] == nil {
				m.msg = i18n.T("msg.nolog")
				break
			}
			m.openRun(c.ID)
		}
	case "c":
		if c := m.curCard(); c != nil {
			if rs := m.runs[c.ID]; rs != nil && rs.run != nil && rs.active() {
				_ = rs.run.Cancel()
				m.msg = i18n.T("msg.cancel")
			}
		}
	case "/":
		m.filterIn.SetValue(m.filter)
		m.mode = modeFilter
		return m, m.filterIn.Focus()
	case "esc":
		m.filter = ""
		m.filterIn.SetValue("")
		m.clamp()
	}
	return m, nil
}

// moveCard moves the selected card to the neighbour column; a card flagged
// auto-run entering an execution column starts its workflow.
func (m *Model) moveCard(d int) tea.Cmd {
	ci, idx, ok := m.cur()
	to := ci + d
	if !ok || to < 0 || to >= len(m.board.Columns) {
		return nil
	}
	id := m.board.Columns[ci].Cards[idx].ID
	m.board.Move(ci, idx, to)
	m.save()
	m.clamp()
	m.focus(id)
	if m.board.Columns[to].AutoRun {
		if c := m.card(id); c != nil && c.AutoRun && c.Workflow != "" {
			return m.startRun(id)
		}
	}
	return nil
}

func (m *Model) reorder(d int) {
	if ci, idx, ok := m.cur(); ok && m.filter == "" {
		id := m.board.Columns[ci].Cards[idx].ID
		if _, ok := m.board.Swap(ci, idx, d); ok {
			m.save()
			m.focus(id)
		}
	}
}

// ---- view ----

// Screen margins (columns / rows).
const padX, padY = 2, 1

func (m *Model) View() string {
	v := m.render()
	ind := strings.Repeat(" ", padX)
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		lines[i] = ind + l
	}
	pad := make([]string, padY)
	v = strings.Join(append(append(pad, lines...), pad...), "\n")
	if m.transparent {
		return v
	}
	return paintPage(v, m.fullW, m.fullH)
}

func (m *Model) render() string {
	switch m.mode {
	case modeForm:
		return m.viewForm()
	case modeRun:
		return m.viewRun()
	case modeHelp:
		return m.viewHelp()
	}
	return m.viewBoard()
}

func (m *Model) viewHelp() string {
	keys := [][2]string{
		{"← ↑ ↓ →", i18n.T("help.nav")},
		{"↵", i18n.T("help.run")},
		{"H L", i18n.T("help.move")},
		{"K J", i18n.T("help.reorder")},
		{"n e d", i18n.T("help.card")},
		{"l", i18n.T("help.logs")}, {"r", i18n.T("help.retry")}, {"c", i18n.T("help.cancel")},
		{"tab", i18n.T("help.tab")},
		{"N R D", i18n.T("help.todo")},
		{"/  esc", i18n.T("help.search")}, {"q", i18n.T("help.quit")},
	}
	var b strings.Builder
	b.WriteString(sBadge.Render("TODO") + " " + sDim.Render(i18n.T("help.title")) + "\n\n")
	for _, k := range keys {
		b.WriteString(sBold.Render(fit(k[0], 9)) + " " + sDim.Render(k[1]) + "\n")
	}
	b.WriteString("\n" + sDim.Render(i18n.T("help.todos", board.Dir(m.dir))))
	b.WriteString("\n" + sDim.Render(i18n.T("help.config", config.Path())))
	return b.String()
}
