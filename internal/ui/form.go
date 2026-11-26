package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"todo/internal/board"
	"todo/internal/i18n"
	"todo/internal/runner"
)

const (
	fTitle = iota
	fColumn
	fWorkflow
	fInput0 // inputs occupy fInput0 .. fInput0+len(inputs)-1, then the auto checkbox
)

const wfRows = 4

type form struct {
	editID    string
	card      board.Card
	title     textinput.Model
	inputs    []*field // one per input declared by the selected workflow
	inNames   []runner.Input
	vals      map[string]string // typed values, kept across workflow changes
	workdir   string
	filter    textinput.Model
	filtering bool
	col       int
	cols      []string
	runCol    string
	wfs       []runner.Workflow // wfs[0] is "no workflow"
	shown     []int
	pos       int
	auto      bool
	focus     int
	err       string
}

const promptRows = 5

// field is a single-line input, or a multi-line one for "prompt" inputs.
type field struct {
	one   textinput.Model
	multi bool
	area  textarea.Model
}

func newField(name, v string) *field {
	f := &field{one: newInput("")}
	if name == "prompt" {
		f.multi = true
		f.area = textarea.New()
		f.area.Prompt = ""
		f.area.ShowLineNumbers = false
		f.area.CharLimit = 0
		f.area.SetHeight(promptRows)
		f.area.SetValue(v)
		return f
	}
	f.one.SetValue(v)
	return f
}

func (f *field) Value() string {
	if f.multi {
		return f.area.Value()
	}
	return f.one.Value()
}

func (f *field) Focus() tea.Cmd {
	if f.multi {
		return f.area.Focus()
	}
	return f.one.Focus()
}

func (f *field) Blur() {
	if f.multi {
		f.area.Blur()
	} else {
		f.one.Blur()
	}
}

func (f *field) SetWidth(w int) {
	if f.multi {
		f.area.SetWidth(w)
		return
	}
	f.one.Width = w
}

func (f *field) View() string {
	if f.multi {
		return f.area.View()
	}
	return f.one.View()
}

func (f *field) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if f.multi {
		f.area, cmd = f.area.Update(msg)
	} else {
		f.one, cmd = f.one.Update(msg)
	}
	return cmd
}

func newInput(placeholder string) textinput.Model {
	t := textinput.New()
	t.Prompt = ""
	t.Placeholder = placeholder
	t.CharLimit = 500
	return t
}

func newForm(m *Model, c *board.Card) *form {
	f := &form{col: m.col, auto: true, vals: map[string]string{}, workdir: m.dir}
	for _, col := range m.board.Columns {
		f.cols = append(f.cols, col.Name)
	}
	if rc := m.board.RunColumn(); rc >= 0 {
		f.runCol = m.board.Columns[rc].Name
	}
	f.wfs = append([]runner.Workflow{{}}, m.wfs...)
	f.title = newInput(i18n.T("form.titleph"))
	f.filter = newInput(i18n.T("form.filterph"))
	if c != nil {
		f.editID, f.card, f.auto = c.ID, *c, c.AutoRun
		f.title.SetValue(c.Title)
		for k, v := range c.Inputs {
			f.vals[k] = v
		}
		f.col, _, _ = m.board.Find(c.ID)
		if c.Workflow != "" && !hasWorkflow(f.wfs, c.Workflow) {
			f.wfs = append(f.wfs, m.workflow(c.Workflow))
		}
	}
	f.refilter(c)
	return f
}

func hasWorkflow(ws []runner.Workflow, name string) bool {
	for _, w := range ws {
		if w.Name == name {
			return true
		}
	}
	return false
}

// refilter recomputes the visible workflows; when c is set the selection lands on its workflow.
func (f *form) refilter(c *board.Card) {
	q := strings.ToLower(f.filter.Value())
	f.shown = f.shown[:0]
	for i, w := range f.wfs {
		if q == "" || (i > 0 && strings.Contains(strings.ToLower(w.Name), q)) {
			f.shown = append(f.shown, i)
		}
	}
	f.pos = 0
	if c != nil {
		for p, i := range f.shown {
			if f.wfs[i].Name == c.Workflow {
				f.pos = p
			}
		}
	}
	f.syncInputs()
}

func (f *form) autoIdx() int { return fInput0 + len(f.inputs) }
func (f *form) count() int   { return f.autoIdx() + 1 }

// syncInputs rebuilds the input fields for the selected workflow, keeping typed values.
func (f *form) syncInputs() {
	for i, in := range f.inNames {
		f.vals[in.Name] = f.inputs[i].Value()
	}
	f.inNames, f.inputs = nil, nil
	if f.pos < len(f.shown) {
		for _, in := range f.wfs[f.shown[f.pos]].Inputs {
			v, ok := f.vals[in.Name]
			switch {
			case ok:
			case in.Name == "workdir":
				v = f.workdir
			default:
				v = in.Default
			}
			f.inNames = append(f.inNames, in)
			f.inputs = append(f.inputs, newField(in.Name, v))
		}
	}
	if f.focus > f.autoIdx() {
		f.focus = f.autoIdx()
	}
}

func (f *form) selected() string {
	if f.pos < len(f.shown) {
		return f.wfs[f.shown[f.pos]].Name
	}
	return ""
}

func (f *form) focusCmd() tea.Cmd {
	f.title.Blur()
	f.filter.Blur()
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	switch {
	case f.focus == fTitle:
		return f.title.Focus()
	case f.focus >= fInput0 && f.focus < f.autoIdx():
		return f.inputs[f.focus-fInput0].Focus()
	case f.focus == fWorkflow && f.filtering:
		return f.filter.Focus()
	}
	return nil
}

func (f *form) move(d int) tea.Cmd {
	n := f.count()
	f.focus = (f.focus + n + d) % n
	f.filtering = false
	return f.focusCmd()
}

func (m *Model) submitForm() tea.Cmd {
	f := m.form
	title := strings.TrimSpace(f.title.Value())
	if title == "" {
		f.err = i18n.T("form.err.title")
		f.focus = fTitle
		return f.focusCmd()
	}
	c := f.card
	c.Title = title
	c.Workflow = f.selected()
	if len(f.inputs) > 0 {
		c.Inputs = map[string]string{}
		for i, in := range f.inNames {
			v := strings.TrimSpace(f.inputs[i].Value())
			if v == "" && in.Required {
				f.err = i18n.T("form.err.required", in.Name)
				f.focus = fInput0 + i
				return f.focusCmd()
			}
			if v != "" {
				c.Inputs[in.Name] = v
			}
		}
		if len(c.Inputs) == 0 {
			c.Inputs = nil
		}
	}
	c.AutoRun = f.auto
	m.mode = modeBoard
	if f.editID != "" {
		if p := m.card(f.editID); p != nil {
			*p = c
		}
		m.save()
		return nil
	}
	m.board.Add(f.col, c)
	id := m.board.Columns[f.col].Cards[len(m.board.Columns[f.col].Cards)-1].ID
	m.save()
	m.clamp()
	m.focus(id)
	if m.board.Columns[f.col].AutoRun && c.AutoRun && c.Workflow != "" {
		return m.startRun(id)
	}
	return nil
}

func (m *Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	f.err = ""
	k := msg.String()
	switch k {
	case "esc":
		if f.filtering {
			f.filtering = false
			f.filter.SetValue("")
			f.refilter(nil)
			return m, f.focusCmd()
		}
		m.mode = modeBoard
		return m, nil
	case "tab":
		return m, f.move(1)
	case "shift+tab":
		return m, f.move(-1)
	case "ctrl+v":
		if i := f.focus - fInput0; i >= 0 && i < len(f.inputs) && f.inputs[i].multi {
			return m, pasteImageCmd(m.dir, pasteForm, i)
		}
	case "enter", "ctrl+s":
		if i := f.focus - fInput0; k == "enter" && i >= 0 && i < len(f.inputs) && f.inputs[i].multi {
			break // newline in a multi-line input
		}
		return m, m.submitForm()
	}
	switch {
	case f.focus == fColumn:
		switch k {
		case "left":
			f.col = (f.col + len(f.cols) - 1) % len(f.cols)
		case "right":
			f.col = (f.col + 1) % len(f.cols)
		case "up":
			return m, f.move(-1)
		case "down":
			return m, f.move(1)
		}
		return m, nil
	case f.focus == fWorkflow:
		switch k {
		case "up":
			if f.pos == 0 {
				return m, f.move(-1)
			}
			f.pos--
			f.syncInputs()
			return m, nil
		case "down":
			if f.pos >= len(f.shown)-1 {
				return m, f.move(1)
			}
			f.pos++
			f.syncInputs()
			return m, nil
		case "/":
			if !f.filtering {
				f.filtering = true
				return m, f.focusCmd()
			}
		}
		if f.filtering {
			var cmd tea.Cmd
			f.filter, cmd = f.filter.Update(msg)
			f.refilter(nil)
			return m, cmd
		}
		return m, nil
	case f.focus == f.autoIdx():
		switch k {
		case " ", "left", "right", "x":
			f.auto = !f.auto
		case "up":
			return m, f.move(-1)
		case "down":
			return m, f.move(1)
		}
		return m, nil
	}
	// text fields
	multi := (*textarea.Model)(nil)
	if i := f.focus - fInput0; i >= 0 && i < len(f.inputs) && f.inputs[i].multi {
		multi = &f.inputs[i].area
	}
	switch k {
	case "up":
		if multi == nil || multi.Line() == 0 {
			return m, f.move(-1)
		}
	case "down":
		if multi == nil || multi.Line() >= multi.LineCount()-1 {
			return m, f.move(1)
		}
	}
	var cmd tea.Cmd
	if f.focus == fTitle {
		f.title, cmd = f.title.Update(msg)
	} else if i := f.focus - fInput0; i >= 0 && i < len(f.inputs) {
		cmd = f.inputs[i].Update(msg)
	}
	return m, cmd
}

// ---- view ----

func (f *form) view(w, h int, transparent bool) []string {
	rows := wfRows
	for {
		out := f.render(w, transparent, rows)
		if len(out) <= h || rows <= 1 {
			return out
		}
		rows--
	}
}

func (f *form) render(w int, transparent bool, rows int) []string {
	mw := 100
	if mw > w-4 {
		mw = w - 4
	}
	inner := mw - 6
	label := func(i int, s string) string {
		if f.focus == i {
			return sBold.Foreground(cPink).Render("▸ " + s)
		}
		return sDim.Render("  " + s)
	}
	bar := func(i int) string {
		if f.focus == i {
			return sBold.Foreground(cLilac).Render("▏")
		}
		return lipgloss.NewStyle().Foreground(cBorder).Render("▏")
	}
	title := i18n.T("form.new")
	create := i18n.T("form.create")
	if f.editID != "" {
		title, create = i18n.T("form.edit"), i18n.T("form.save")
	}
	f.title.Width = inner - 4
	for i := range f.inputs {
		f.inputs[i].SetWidth(inner - 4)
	}
	f.filter.Width = inner - 20

	col := sText.Render(f.cols[f.col])
	if f.focus == fColumn {
		col = sLav.Render("◀ " + f.cols[f.col] + " ▶")
	}

	hdr := i18n.T("form.workflow")
	if f.filtering {
		hdr = i18n.T("form.workflow.filt") + f.filter.View()
	}
	lines := []string{
		sBold.Foreground(cLav).Render(title), "",
		label(fTitle, i18n.T("form.field.title")), bar(fTitle) + " " + f.title.View(), "",
		label(fColumn, i18n.T("form.field.column")), bar(fColumn) + " " + col, "",
		label(fWorkflow, hdr),
	}
	start := 0
	if f.pos >= rows {
		start = f.pos - rows + 1
	}
	for r := 0; r < rows; r++ {
		p := start + r
		if p >= len(f.shown) {
			lines = append(lines, "")
			continue
		}
		w := f.wfs[f.shown[p]]
		name, sum := w.Name, w.Summary()
		if name == "" {
			name = i18n.T("form.none")
		}
		if p == f.pos {
			l := row("▸ "+name, sum, inner-2)
			lines = append(lines, lipgloss.NewStyle().Background(cSelBg).Foreground(cLav).Render(" "+fit(ansiPlain(l), inner-2)+" "))
		} else {
			lines = append(lines, " "+row("  "+sText.Render(name), sDim.Render(sum), inner-2))
		}
	}
	box := "[ ]"
	if f.auto {
		box = sGreen.Render("[x]")
	}
	chk := box + " " + i18n.T("form.auto", f.runCol)
	if f.focus == f.autoIdx() {
		chk = sBold.Foreground(cPink).Render("▸ ") + chk
	} else {
		chk = "  " + chk
	}
	lines = append(lines, "")
	for i, in := range f.inNames {
		name := in.Name
		if in.Required {
			name += " *"
		}
		lines = append(lines, label(fInput0+i, name))
		for _, l := range strings.Split(f.inputs[i].View(), "\n") {
			lines = append(lines, bar(fInput0+i)+" "+l)
		}
		lines = append(lines, "")
	}
	lines = append(lines, chk, "")
	submitKey := "↵"
	if i := f.focus - fInput0; i >= 0 && i < len(f.inputs) && f.inputs[i].multi {
		submitKey = "ctrl+s"
	}
	if f.err != "" {
		lines = append(lines, sRed.Render(f.err))
	} else {
		lines = append(lines, hints(hint("tab", i18n.T("h.nextfield")), hint(submitKey, create), hint("esc", i18n.T("h.cancel"))))
	}
	body := make([]string, len(lines))
	for i, l := range lines {
		body[i] = fit(l, inner)
	}
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cViolet).Padding(0, 2).Width(mw - 2)
	content := strings.Join(body, "\n")
	if !transparent {
		st = st.Background(cCard).BorderBackground(cCard)
		content = keepBg(content, cCard)
	}
	boxed := st.Render(content)
	return strings.Split(boxed, "\n")
}

func ansiPlain(s string) string {
	return ansi.Strip(s)
}

func (m *Model) viewForm() string {
	bg := plainDim(m.viewBoard())
	for len(bg) < m.h {
		bg = append(bg, "")
	}
	fl := m.form.view(m.w, m.h, m.transparent)
	x := (m.w - lipgloss.Width(fl[0])) / 2
	y := (m.h - len(fl)) / 2
	if y < 0 {
		y = 0
	}
	return strings.Join(overlay(bg, fl, x, y), "\n")
}
