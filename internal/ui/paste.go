package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"todo/internal/board"
	"todo/internal/clip"
	"todo/internal/i18n"
)

// imageReader fetches the clipboard image; replaced in tests.
var imageReader = clip.Image

const (
	pasteForm   = iota // multi-line input of the form
	pasteAnswer        // answer box of a conversation step
)

// pasteMsg is the outcome of a ctrl+v: the saved image path, or nothing when
// the clipboard holds no image (the key is then handled as a plain text paste).
type pasteMsg struct {
	target, idx int
	path        string
	err         error
	noImage     bool
}

// pasteImageCmd reads the clipboard off the UI loop and stores an image under .todo/attachments.
func pasteImageCmd(dir string, target, idx int) tea.Cmd {
	return func() tea.Msg {
		msg := pasteMsg{target: target, idx: idx}
		png, err := imageReader()
		if err != nil {
			msg.noImage = true
			return msg
		}
		msg.path, msg.err = board.SaveAttachment(dir, png)
		return msg
	}
}

func (m *Model) onPaste(p pasteMsg) tea.Cmd {
	switch {
	case p.noImage:
		// no image: behave like a regular ctrl+v (text paste)
		return m.pasteText(p)
	case p.err != nil:
		m.msg = i18n.T("err.paste", firstLine(p.err.Error()))
		return nil
	}
	switch p.target {
	case pasteForm:
		if m.mode == modeForm && p.idx < len(m.form.inputs) {
			m.form.inputs[p.idx].area.InsertString(p.path)
		}
	case pasteAnswer:
		if rs := m.runs[m.runCard]; m.mode == modeRun && rs.asking() {
			m.answer.SetValue(m.answer.Value() + p.path)
			m.answer.CursorEnd()
		}
	}
	m.msg = i18n.T("msg.pasted", p.path)
	return nil
}

// pasteText forwards ctrl+v to the focused input, which pastes the text clipboard.
func (m *Model) pasteText(p pasteMsg) tea.Cmd {
	k := tea.KeyMsg{Type: tea.KeyCtrlV}
	switch p.target {
	case pasteForm:
		if m.mode == modeForm && p.idx < len(m.form.inputs) {
			return m.form.inputs[p.idx].Update(k)
		}
	case pasteAnswer:
		if m.mode == modeRun {
			var cmd tea.Cmd
			m.answer, cmd = m.answer.Update(k)
			return cmd
		}
	}
	return nil
}
