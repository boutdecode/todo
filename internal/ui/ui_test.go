package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todo/internal/board"
	"todo/internal/config"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drain executes a command chain synchronously until no command is left.
func drain(m *Model, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		var next tea.Cmd
		_, next = m.Update(msg)
		cmd = next
	}
}

func newModel(t *testing.T, script string) *Model {
	dir := t.TempDir()
	bin := filepath.Join(dir, "awf")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := board.Load(dir, nil)
	b.Add(0, board.Card{Title: "Ma tâche", Workflow: "wf"})
	cfg := &config.Config{AWF: bin, Columns: board.DefaultColumns()}
	m := New(dir, bin, cfg, b)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestAutoRunMovesCard(t *testing.T) {
	m := newModel(t, `echo "Workflow started: r1"; echo "[OUT] hi"; echo ok`)
	_, cmd := m.Update(key("L")) // Backlog -> Doing (auto-run)
	drain(m, cmd)
	if n := len(m.board.Columns[2].Cards); n != 1 {
		t.Fatalf("card should be in Done: %+v", m.board.Columns)
	}
	if lr := m.board.Columns[2].Cards[0].LastRun; lr == nil || lr.Status != "completed" || lr.ID != "r1" {
		t.Fatalf("last run: %+v", lr)
	}
	if _, err := os.Stat(board.Path(m.dir)); err != nil {
		t.Fatal("board not saved")
	}
}

func TestManualRunFailure(t *testing.T) {
	m := newModel(t, `echo "Workflow started: r2"; echo "Workflow failed: boom"; exit 1`)
	m.Update(key("l")) // Doing column is empty; go back
	m.Update(key("h"))
	_, cmd := m.Update(key("L"))
	drain(m, cmd)
	if len(m.board.Columns[3].Cards) != 1 {
		t.Fatalf("card should be in Failed: %+v", m.board.Columns)
	}
}

func TestFormCreatesCard(t *testing.T) {
	m := newModel(t, "true")
	_, cmd := m.Update(key("n"))
	_ = cmd
	for _, r := range "Nouvelle" {
		m.Update(key(string(r)))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != modeBoard || len(m.board.Columns[0].Cards) != 2 {
		t.Fatalf("mode=%v cards=%d", m.mode, len(m.board.Columns[0].Cards))
	}
}

func TestViewFitsTerminal(t *testing.T) {
	m := newModel(t, "true")
	v := m.View()
	if lipgloss.Height(v) > 24 || lipgloss.Width(v) > 80 {
		t.Fatalf("view %dx%d exceeds 80x24\n%s", lipgloss.Width(v), lipgloss.Height(v), v)
	}
	if !strings.Contains(v, "BACKLOG") || !strings.Contains(v, "Ma tâche") {
		t.Fatalf("missing content:\n%s", v)
	}
	m.Update(key("?"))
	if !strings.Contains(m.View(), "aide") {
		t.Fatal("help")
	}
}
