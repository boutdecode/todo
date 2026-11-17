package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todo/internal/board"
)

const (
	fTitle = iota
	fDesc
	fWorkflow
	fInputs
	fCount
)

type form struct {
	editID string // empty when creating
	col    int
	inputs [fCount]textinput.Model // fWorkflow is unused (selector)
	wfs    []string                // "" first = no workflow
	wfIdx  int
	focus  int
	card   board.Card
}

func newForm(m *Model, c *board.Card) *form {
	f := &form{col: m.col, wfs: append([]string{""}, m.wfs...)}
	for i := range f.inputs {
		f.inputs[i] = textinput.New()
		f.inputs[i].Prompt = ""
		f.inputs[i].CharLimit = 500
	}
	f.inputs[fTitle].Placeholder = "Titre"
	f.inputs[fDesc].Placeholder = "Description (optionnelle)"
	f.inputs[fInputs].Placeholder = "clé=valeur, clé2=valeur2"
	if c != nil {
		f.editID = c.ID
		f.card = *c
		f.inputs[fTitle].SetValue(c.Title)
		f.inputs[fDesc].SetValue(c.Description)
		f.inputs[fInputs].SetValue(formatInputs(c.Inputs))
		for i, w := range f.wfs {
			if w == c.Workflow {
				f.wfIdx = i
			}
		}
		if c.Workflow != "" && f.wfIdx == 0 { // workflow not found on disk: keep it
			f.wfs = append(f.wfs, c.Workflow)
			f.wfIdx = len(f.wfs) - 1
		}
	}
	return f
}

func formatInputs(in map[string]string) string {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + in[k]
	}
	return strings.Join(parts, ", ")
}

func parseInputs(s string) map[string]string {
	out := map[string]string{}
	for _, p := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if k = strings.TrimSpace(k); ok && k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f *form) focusCmd() tea.Cmd {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	if f.focus == fWorkflow {
		return nil
	}
	return f.inputs[f.focus].Focus()
}

func (m *Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	switch msg.String() {
	case "esc":
		m.mode = modeBoard
		return m, nil
	case "tab", "down":
		f.focus = (f.focus + 1) % fCount
		return m, f.focusCmd()
	case "shift+tab", "up":
		f.focus = (f.focus + fCount - 1) % fCount
		return m, f.focusCmd()
	case "ctrl+s", "enter":
		if msg.String() == "enter" && f.focus != fInputs {
			f.focus++
			return m, f.focusCmd()
		}
		title := strings.TrimSpace(f.inputs[fTitle].Value())
		if title == "" {
			f.focus = fTitle
			m.msg = "le titre est obligatoire"
			return m, f.focusCmd()
		}
		c := f.card
		c.Title = title
		c.Description = strings.TrimSpace(f.inputs[fDesc].Value())
		c.Workflow = f.wfs[f.wfIdx]
		c.Inputs = parseInputs(f.inputs[fInputs].Value())
		if f.editID != "" {
			if p := m.card(f.editID); p != nil {
				*p = c
			}
		} else {
			m.board.Add(f.col, c)
			m.col = f.col
			m.focus(m.board.Columns[f.col].Cards[len(m.board.Columns[f.col].Cards)-1].ID)
		}
		m.save()
		m.clamp()
		m.mode = modeBoard
		return m, nil
	}
	if f.focus == fWorkflow {
		switch msg.String() {
		case "left", "h":
			f.wfIdx = (f.wfIdx + len(f.wfs) - 1) % len(f.wfs)
		case "right", "l", " ":
			f.wfIdx = (f.wfIdx + 1) % len(f.wfs)
		}
		return m, nil
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return m, cmd
}

func (f *form) view(w, h int) string {
	label := func(i int, s string) string {
		if f.focus == i {
			return sBold.Foreground(cPink).Render("▸ " + s)
		}
		return sDim.Render("  " + s)
	}
	wf := f.wfs[f.wfIdx]
	if wf == "" {
		wf = "(aucun)"
	}
	wfLine := sText.Render("◀ " + wf + " ▶")
	if f.focus != fWorkflow {
		wfLine = sPurple.Render(wf)
	}
	title := "Nouvelle carte"
	if f.editID != "" {
		title = "Éditer la carte"
	}
	body := strings.Join([]string{
		label(fTitle, "Titre"), "  " + f.inputs[fTitle].View(), "",
		label(fDesc, "Description"), "  " + f.inputs[fDesc].View(), "",
		label(fWorkflow, "Workflow  (←→ pour changer)"), "  " + wfLine, "",
		label(fInputs, "Inputs"), "  " + f.inputs[fInputs].View(),
	}, "\n")
	bw := w - 4
	if bw > 80 {
		bw = 80
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cAccent).Padding(1, 2).Width(bw).Render(body)
	foot := hint("tab", "champ suivant") + " " + hint("ctrl+s", "enregistrer") + " " + hint("esc", "annuler")
	return sLogo.Render("todo") + " " + sBold.Render(title) + "\n" + box + "\n" + foot
}
