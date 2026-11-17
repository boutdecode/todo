package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"todo/internal/board"
	"todo/internal/config"
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
)

// runState is the in-memory state of a workflow run attached to a card.
type runState struct {
	run      *runner.Run
	starting bool
	lines    []string
	status   string // running|completed|failed|cancelled
	ask      string
	err      string
}

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
)

// Model is the root Bubble Tea model.
type Model struct {
	dir, bin string
	cfg      *config.Config
	board    *board.Board
	wfs      []string

	mode          mode
	col           int
	sel           []int // selected position (in the visible list) per column
	filter        string
	filterIn      textinput.Model
	w, h          int
	msg           string
	runs          map[string]*runState
	form          *form
	runCard       string
	vp            viewport.Model
	answer        textinput.Model
	confirmCardID string
}

// New builds the model for a project directory.
func New(dir, bin string, cfg *config.Config, b *board.Board) *Model {
	fi := textinput.New()
	fi.Prompt = "/ "
	ans := textinput.New()
	ans.Prompt = "› "
	return &Model{
		dir: dir, bin: bin, cfg: cfg, board: b,
		wfs:  runner.Workflows(dir),
		sel:  make([]int, len(b.Columns)),
		runs: map[string]*runState{}, filterIn: fi, answer: ans,
		w: 100, h: 30,
	}
}

func (m *Model) Init() tea.Cmd { return nil }

// ---- helpers ----

func (m *Model) save() {
	if err := m.board.Save(m.dir); err != nil {
		m.msg = "sauvegarde impossible : " + err.Error()
	}
}

// vis returns the indices of the cards of column ci that match the filter.
func (m *Model) vis(ci int) []int {
	var out []int
	f := strings.ToLower(m.filter)
	for i, c := range m.board.Columns[ci].Cards {
		if f == "" || strings.Contains(strings.ToLower(c.Title+" "+c.Workflow+" "+c.Description), f) {
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
			ev = runner.Event{Kind: runner.KindDone, Status: "failed", Error: "flux interrompu"}
		}
		return runEventMsg{card: id, ev: ev, ch: ch}
	}
}

// startRun starts the workflow of card id unless one is already running.
func (m *Model) startRun(id string) tea.Cmd {
	c := m.card(id)
	if c == nil {
		return nil
	}
	if c.Workflow == "" {
		m.msg = "cette carte n'a pas de workflow"
		return nil
	}
	if rs := m.runs[id]; rs != nil && (rs.starting || rs.status == board.StatusRunning) {
		m.msg = "run déjà en cours"
		return nil
	}
	m.runs[id] = &runState{starting: true, status: board.StatusRunning}
	c.LastRun = &board.LastRun{Status: board.StatusRunning, At: time.Now()}
	m.save()
	m.msg = "démarrage de " + c.Workflow + "…"
	return startCmd(m.bin, m.dir, *c)
}

func (m *Model) onEvent(msg runEventMsg) tea.Cmd {
	rs := m.runs[msg.card]
	if rs == nil {
		return nil
	}
	ev := msg.ev
	switch ev.Kind {
	case runner.KindStepStarted:
		rs.lines = append(rs.lines, sPurple.Render("→ "+ev.Step))
	case runner.KindStepDone:
		st := sOK
		if ev.Status != "completed" {
			st = sBad
		}
		rs.lines = append(rs.lines, st.Render(fmt.Sprintf("✓ %s: %s", ev.Step, ev.Status)))
	case runner.KindOutput:
		if ev.Stream == "err" {
			rs.lines = append(rs.lines, sBad.Render(ev.Line))
		} else {
			rs.lines = append(rs.lines, ev.Line)
		}
	case runner.KindAsk:
		rs.ask = ev.Prompt
	case runner.KindDone:
		rs.status = ev.Status
		rs.ask = ""
		rs.err = ev.Error
		m.finish(msg.card, ev)
		m.refreshRun()
		return nil
	}
	m.refreshRun()
	return waitEvent(msg.card, msg.ch)
}

// finish records the result on the card and applies the column's OnSuccess/OnFailure.
func (m *Model) finish(id string, ev runner.Event) {
	ci, idx, ok := m.board.Find(id)
	if !ok {
		return
	}
	c := &m.board.Columns[ci].Cards[idx]
	runID := ""
	if rs := m.runs[id]; rs != nil && rs.run != nil {
		runID = rs.run.ID
	}
	c.LastRun = &board.LastRun{ID: runID, Status: ev.Status, At: time.Now()}
	target := ""
	switch ev.Status {
	case board.StatusCompleted:
		target = m.board.Columns[ci].OnSuccess
	case board.StatusFailed:
		target = m.board.Columns[ci].OnFailure
	}
	m.msg = fmt.Sprintf("%s : %s", c.Title, ev.Status)
	if to := m.board.ColumnIndex(target); to >= 0 && to != ci {
		m.board.Move(ci, idx, to)
	}
	m.save()
	m.clamp()
}

func (m *Model) runContent() string {
	rs := m.runs[m.runCard]
	if rs == nil {
		return ""
	}
	out := strings.Join(rs.lines, "\n")
	if rs.err != "" {
		out += "\n" + sBad.Render(rs.err)
	}
	return out
}

func (m *Model) refreshRun() {
	if m.mode != modeRun {
		return
	}
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(m.runContent())
	if atBottom {
		m.vp.GotoBottom()
	}
}

// ---- update ----

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.vp.Width, m.vp.Height = m.w-4, m.h-6
		return m, nil
	case runStartedMsg:
		rs := m.runs[msg.card]
		if rs == nil {
			return m, nil
		}
		rs.starting = false
		if msg.err != nil {
			rs.status = board.StatusFailed
			rs.err = msg.err.Error()
			m.finish(msg.card, runner.Event{Kind: runner.KindDone, Status: "failed", Error: rs.err})
			m.msg = "échec du lancement : " + firstLine(rs.err)
			return m, nil
		}
		rs.run = msg.run
		m.msg = "run " + msg.run.ID
		return m, waitEvent(msg.card, msg.run.Events)
	case runEventMsg:
		return m, m.onEvent(msg)
	case tea.KeyMsg:
		switch m.mode {
		case modeForm:
			return m.updateForm(msg)
		case modeRun:
			return m.updateRun(msg)
		case modeConfirm:
			if msg.String() == "y" {
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

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func (m *Model) updateBoard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.msg = ""
	nc := len(m.board.Columns)
	switch msg.String() {
	case "q", "ctrl+c":
		for _, rs := range m.runs {
			if rs.run != nil && rs.status == board.StatusRunning {
				_ = rs.run.Cancel()
			}
		}
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "left", "h":
		if m.col > 0 {
			m.col--
		}
	case "right", "l":
		if m.col < nc-1 {
			m.col++
		}
	case "up", "k":
		if m.sel[m.col] > 0 {
			m.sel[m.col]--
		}
	case "down", "j":
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
		if ci, idx, ok := m.cur(); ok {
			c := m.board.Columns[ci].Cards[idx]
			m.form = newForm(m, &c)
			m.mode = modeForm
			return m, m.form.focusCmd()
		}
	case "d":
		if ci, idx, ok := m.cur(); ok {
			m.confirmCardID = m.board.Columns[ci].Cards[idx].ID
			m.mode = modeConfirm
		}
	case "x":
		if ci, idx, ok := m.cur(); ok {
			return m, m.startRun(m.board.Columns[ci].Cards[idx].ID)
		}
	case "c":
		if ci, idx, ok := m.cur(); ok {
			if rs := m.runs[m.board.Columns[ci].Cards[idx].ID]; rs != nil && rs.run != nil && rs.status == board.StatusRunning {
				_ = rs.run.Cancel()
				m.msg = "annulation…"
			}
		}
	case "enter":
		if ci, idx, ok := m.cur(); ok {
			id := m.board.Columns[ci].Cards[idx].ID
			if m.runs[id] == nil {
				m.msg = "aucune sortie : lancez le workflow avec x"
				break
			}
			m.runCard = id
			m.mode = modeRun
			m.vp.Width, m.vp.Height = m.w-4, m.h-6
			m.vp.SetContent(m.runContent())
			m.vp.GotoBottom()
		}
	case "/":
		m.filterIn.SetValue(m.filter)
		m.mode = modeFilter
		return m, m.filterIn.Focus()
	case "esc":
		m.filter = ""
		m.clamp()
	}
	return m, nil
}

// moveCard moves the selected card to the neighbour column; auto-run columns start it.
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
		if c := m.card(id); c != nil && c.Workflow != "" {
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
			rs.lines = append(rs.lines, sDim.Render("› "+txt))
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
		if rs != nil && rs.run != nil && rs.status == board.StatusRunning {
			_ = rs.run.Cancel()
		}
	case "r":
		if rs != nil && rs.status != board.StatusRunning && !rs.starting {
			id := m.runCard
			delete(m.runs, id)
			cmd := m.startRun(id)
			m.vp.SetContent("")
			return m, cmd
		}
	default:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ---- view ----

func (m *Model) View() string {
	switch m.mode {
	case modeForm:
		return m.form.view(m.w, m.h)
	case modeRun:
		return m.viewRun()
	case modeHelp:
		return m.viewHelp()
	}
	return m.viewBoard()
}

func statusBadge(rs *runState, c board.Card) string {
	st := ""
	if rs != nil {
		st = rs.status
	} else if c.LastRun != nil {
		st = c.LastRun.Status
		if st == board.StatusRunning { // stale: left over from a previous session
			st = board.StatusCancelled
		}
	}
	switch st {
	case board.StatusRunning:
		return sRun.Render("⏵")
	case board.StatusCompleted:
		return sOK.Render("✓")
	case board.StatusFailed:
		return sBad.Render("✗")
	case board.StatusCancelled:
		return sDim.Render("⊘")
	}
	return ""
}

func (m *Model) viewBoard() string {
	n := len(m.board.Columns)
	header := sLogo.Render("todo") + " " + sDim.Render(shortDir(m.dir))
	if m.filter != "" {
		header += "  " + sRun.Render("filtre: "+m.filter)
	}
	areaH := m.h - 2
	cw := (m.w - (n - 1)) / n
	cols := make([]string, n)
	for ci, col := range m.board.Columns {
		cols[ci] = m.renderColumn(ci, col, cw, areaH)
	}
	parts := make([]string, 0, 2*n)
	for i, c := range cols {
		if i > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, c)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	var foot string
	switch m.mode {
	case modeFilter:
		foot = m.filterIn.View()
	case modeConfirm:
		foot = sBad.Render("Supprimer cette carte ? ") + hint("y", "oui") + " " + hint("autre", "non")
	default:
		foot = hint("n", "nouv.") + " " + hint("e", "éditer") + " " + hint("x", "run") + " " +
			hint("H/L", "déplacer") + " " + hint("enter", "sortie") + " " + hint("?", "aide") + " " + hint("q", "quitter")
		if m.msg != "" {
			foot = sRun.Render(fit(m.msg, m.w))
		}
	}
	return header + "\n" + body + "\n" + ansi.Truncate(foot, m.w, "")
}

func shortDir(d string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(d, home) {
		return "~" + strings.TrimPrefix(d, home)
	}
	return d
}

func (m *Model) renderColumn(ci int, col board.Column, cw, areaH int) string {
	focused := ci == m.col
	border := cBorder
	if focused {
		border = cAccent
	}
	inner := cw - 2
	title := strings.ToUpper(col.Name)
	if col.AutoRun {
		title += " ⚡"
	}
	vis := m.vis(ci)
	head := sBold.Foreground(cPurple).Render(fit(fmt.Sprintf("%s (%d)", title, len(vis)), inner))

	const cardH = 4
	capacity := (areaH - 2 - 1) / cardH
	if capacity < 1 {
		capacity = 1
	}
	start := 0
	if m.sel[ci] >= capacity {
		start = m.sel[ci] - capacity + 1
	}
	lines := []string{head}
	for p := start; p < len(vis) && p < start+capacity; p++ {
		c := col.Cards[vis[p]]
		bc, fg := cBorder, cText
		if focused && p == m.sel[ci] {
			bc = cPink
		}
		sub := sDim.Render("—")
		if c.Workflow != "" {
			sub = sPurple.Render("⚙ " + c.Workflow)
		}
		badge := statusBadge(m.runs[c.ID], c)
		top := fit(c.Title, inner-4)
		if badge != "" {
			top = fit(c.Title, inner-6) + " " + badge
		}
		content := lipgloss.NewStyle().Foreground(fg).Render(top) + "\n" + fit(sub, inner-4)
		box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(bc).Width(inner - 2).Render(content)
		lines = append(lines, box)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Width(inner).Height(areaH - 2).Render(strings.Join(lines, "\n"))
}

func (m *Model) viewRun() string {
	c := m.card(m.runCard)
	title, wf := "?", ""
	if c != nil {
		title, wf = c.Title, c.Workflow
	}
	rs := m.runs[m.runCard]
	st := ""
	if rs != nil {
		st = rs.status
		if rs.starting {
			st = "démarrage"
		}
	}
	head := sLogo.Render("run") + " " + sBold.Render(title) + " " + sPurple.Render("⚙ "+wf) + " " + sRun.Render(st)
	body := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cBorder).
		Width(m.w - 2).Height(m.h - 4).Render(m.vp.View())
	foot := hint("↑↓", "défiler") + " " + hint("c", "annuler") + " " + hint("r", "relancer") + " " + hint("esc", "retour")
	if rs != nil && rs.ask != "" {
		foot = sRun.Render(rs.ask) + " " + m.answer.View()
	}
	return head + "\n" + body + "\n" + foot
}

func (m *Model) viewHelp() string {
	keys := [][2]string{
		{"←→ h l", "changer de colonne"}, {"↑↓ j k", "changer de carte"},
		{"H L", "déplacer la carte (colonne auto-run : lance le workflow)"},
		{"K J", "réordonner dans la colonne"}, {"n / e / d", "nouvelle / éditer / supprimer"},
		{"x", "exécuter le workflow de la carte"}, {"c", "annuler le run"},
		{"enter", "voir la sortie du run"}, {"/  esc", "filtrer / effacer le filtre"},
		{"q", "quitter (annule les runs en cours)"},
	}
	var b strings.Builder
	b.WriteString(sLogo.Render("todo") + " " + sDim.Render("aide") + "\n\n")
	for _, k := range keys {
		b.WriteString(sKey.Render(fit(k[0], 8)) + " " + k[1] + "\n")
	}
	b.WriteString("\n" + sDim.Render("Board : "+board.Path(m.dir)))
	b.WriteString("\n" + sDim.Render("Config : "+config.Path()))
	b.WriteString("\n\n" + sDim.Render("Colonnes : "+columnsSummary(m.board)))
	return b.String()
}

func columnsSummary(b *board.Board) string {
	names := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		names[i] = c.Name
	}
	return strings.Join(names, " · ")
}

// Workflows returns the workflow names known to the model (sorted).
func (m *Model) Workflows() []string {
	out := append([]string(nil), m.wfs...)
	sort.Strings(out)
	return out
}
