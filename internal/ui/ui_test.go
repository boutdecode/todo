package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"todo/internal/board"
	"todo/internal/config"
	"todo/internal/i18n"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drain executes a command chain synchronously (ticks are skipped).
func drain(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			drain(m, c)
		}
		return
	case tickMsg:
		return
	default:
		_, next := m.Update(msg)
		drain(m, next)
	}
}

func newModel(t *testing.T, script string) *Model {
	dir := t.TempDir()
	bin := filepath.Join(dir, "awf")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	bs, _ := board.LoadAll(dir, nil)
	b := bs[0]
	b.Add(1, board.Card{Title: "Ma tâche", Workflow: "wf", AutoRun: true}) // "To do"
	cfg := &config.Config{AWF: bin, Columns: board.DefaultColumns()}
	m := New(dir, bin, cfg, bs)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.col = 1
	return m
}

const doneIdx, runIdx = 3, 2

func TestEnterLaunchesAndMovesToDone(t *testing.T) {
	m := newModel(t, `echo "Workflow started: r1"; echo "[OUT] hi"; echo ok`)
	_, cmd := m.Update(key("enter"))
	if len(m.board.Columns[runIdx].Cards) != 1 {
		t.Fatal("card should move to the run column first")
	}
	drain(m, cmd)
	if n := len(m.board.Columns[doneIdx].Cards); n != 1 {
		t.Fatalf("card should be in Done: %+v", m.board.Columns)
	}
	if lr := m.board.Columns[doneIdx].Cards[0].LastRun; lr == nil || lr.Status != "completed" || lr.ID != "r1" {
		t.Fatalf("last run: %+v", lr)
	}
	if _, err := os.Stat(board.Path(m.dir)); err != nil {
		t.Fatal("board not saved")
	}
}

func TestAutoRunOnlyForFlaggedCards(t *testing.T) {
	m := newModel(t, `echo "Workflow started: r1"; echo ok`)
	m.board.Columns[1].Cards[0].AutoRun = false
	_, cmd := m.Update(key("L"))
	if cmd != nil {
		t.Fatal("unflagged card must not auto-run")
	}
	m = newModel(t, `echo "Workflow started: r1"; echo ok`)
	_, cmd = m.Update(key("L"))
	drain(m, cmd)
	if len(m.board.Columns[doneIdx].Cards) != 1 {
		t.Fatalf("flagged card should have run: %+v", m.board.Columns)
	}
}

func TestFailureShowsInDoneColumn(t *testing.T) {
	m := newModel(t, `echo "Workflow started: r2"; echo "Workflow failed: boom"; exit 1`)
	_, cmd := m.Update(key("enter"))
	drain(m, cmd)
	cards := m.board.Columns[doneIdx].Cards
	if len(cards) != 1 || cards[0].LastRun.Status != "failed" {
		t.Fatalf("%+v", m.board.Columns)
	}
	m.col = doneIdx
	if v := m.View(); !strings.Contains(v, "✗") || !strings.Contains(v, "r to retry") {
		t.Fatalf("failure not shown:\n%s", v)
	}
}

func TestFormCreatesCard(t *testing.T) {
	m := newModel(t, "true")
	m.Update(key("n"))
	for _, r := range "New" {
		m.Update(key(string(r)))
	}
	m.Update(key("ctrl+s"))
	if m.mode != modeBoard || len(m.board.Columns[1].Cards) != 2 {
		t.Fatalf("mode=%v cards=%d", m.mode, len(m.board.Columns[1].Cards))
	}
	if !m.board.Columns[1].Cards[1].AutoRun {
		t.Fatal("auto-run checkbox defaults to on")
	}
}

func TestViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {140, 40}} {
		m := newModel(t, "true")
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		check := func(name string) {
			v := m.View()
			if lipgloss.Width(v) > size[0] || lipgloss.Height(v) > size[1] {
				t.Fatalf("%s %v: view %dx%d\n%s", name, size, lipgloss.Width(v), lipgloss.Height(v), v)
			}
		}
		check("board")
		if v := m.View(); !strings.Contains(v, "BACKLOG") && !strings.Contains(v, "Backlog") || !strings.Contains(v, "In progress") || !strings.Contains(v, "enter ↵") {
			t.Fatalf("missing board content:\n%s", v)
		}
		m.Update(key("n"))
		check("form")
		if !strings.Contains(m.View(), "New task") {
			t.Fatal("form title")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m.Update(key("/"))
		for _, r := range "ma" {
			m.Update(key(string(r)))
		}
		check("filter")
		if !strings.Contains(m.View(), "1 result") {
			t.Fatalf("search bar:\n%s", m.View())
		}
	}
}

func TestRunViewAndLogs(t *testing.T) {
	m := newModel(t, `echo "Workflow started: r3"; echo "  → build"; echo "[OUT] hello"; echo "  ✓ build: completed"; sleep 1`)
	_, cmd := m.Update(key("enter"))
	// start, then consume a few events without waiting for the end
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			if sm, ok := c().(runStartedMsg); ok {
				m.Update(sm)
			}
		}
	}
	m.Update(key("l"))
	if m.mode != modeRun {
		t.Fatal("l opens the logs")
	}
	v := m.View()
	if !strings.Contains(v, "Steps") || !strings.Contains(v, "Logs (stream)") || !strings.Contains(v, "back to board") {
		t.Fatalf("run view:\n%s", v)
	}
	m.Update(key("c"))
}

func TestPageBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	m := newModel(t, "true")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	bg := bgSeq(cPage)
	if bg == "" {
		t.Fatal("no bg sequence under truecolor")
	}
	check := func(name string) {
		lines := strings.Split(m.View(), "\n")
		if len(lines) != 30 {
			t.Fatalf("%s: %d lines", name, len(lines))
		}
		for i, l := range lines {
			if !strings.HasPrefix(l, bg) || lipgloss.Width(l) != 100 {
				t.Fatalf("%s line %d not fully painted (w=%d)", name, i, lipgloss.Width(l))
			}
			body := strings.TrimSuffix(l, "\x1b[0m")
			if strings.Count(body, "\x1b[0m") != strings.Count(body, "\x1b[0m"+bg+fgSeq(cText)) {
				t.Fatalf("%s line %d: reset without background", name, i)
			}
		}
	}
	check("board")
	m.Update(key("n"))
	check("form")
	m.SetTransparent(true)
	if strings.Contains(m.View(), bg) {
		t.Fatal("transparent must not paint the page")
	}
}

func typeName(m *Model, name string) {
	for _, r := range name {
		m.Update(key(string(r)))
	}
	m.Update(key("enter"))
}

func TestTabsCreateSwitchDelete(t *testing.T) {
	m := newModel(t, "true")
	m.Update(key("N"))
	typeName(m, "Sprint 2")
	if len(m.boards) != 2 || m.active != 1 || m.board.Title() != "Sprint 2" {
		t.Fatalf("create: %d %d", len(m.boards), m.active)
	}
	if _, err := os.Stat(filepath.Join(m.dir, ".todo", "sprint-2.yaml")); err != nil {
		t.Fatal(err)
	}
	if v := m.View(); !strings.Contains(v, "Sprint 2") || !strings.Contains(v, "Board") {
		t.Fatalf("tab bar:\n%s", v)
	}
	m.Update(key("shift+tab"))
	if m.active != 0 || m.col != 1 {
		t.Fatalf("cursor not restored: %d col %d", m.active, m.col)
	}
	m.Update(key("tab"))
	m.Update(key("tab"))
	if m.active != 0 {
		t.Fatal("tab should cycle")
	}
	m.Update(key("tab"))
	m.Update(key("R"))
	m.nameIn.SetValue("")
	typeName(m, "Idées")
	if m.board.Title() != "Idées" {
		t.Fatal("rename")
	}
	m.Update(key("D"))
	m.Update(key("y"))
	if len(m.boards) != 1 || m.board.Title() != "Board" {
		t.Fatalf("delete: %d", len(m.boards))
	}
	if _, err := os.Stat(filepath.Join(m.dir, ".todo", "sprint-2.yaml")); err == nil {
		t.Fatal("file should be removed")
	}
	m.Update(key("D"))
	m.Update(key("y"))
	if len(m.boards) != 1 {
		t.Fatal("last todo must stay")
	}
}

func TestRunFinishesInBackgroundTodo(t *testing.T) {
	m := newModel(t, `sleep 0.2; echo "Workflow started: r9"; echo ok`)
	id := m.board.Columns[1].Cards[0].ID
	_, cmd := m.Update(key("enter"))
	m.Update(key("N"))
	typeName(m, "Autre")
	if m.active != 1 {
		t.Fatal("switch")
	}
	drain(m, cmd)
	if m.active != 1 || len(m.board.Columns[doneIdx].Cards) != 0 {
		t.Fatal("finish must not touch the active todo")
	}
	if ci, _, ok := m.boards[0].Find(id); !ok || ci != doneIdx {
		t.Fatalf("card should be in Done of the first todo (col %d)", ci)
	}
	data, _ := os.ReadFile(m.boards[0].File)
	if !strings.Contains(string(data), "completed") {
		t.Fatal("first todo not saved")
	}
}

func TestFrenchInterface(t *testing.T) {
	i18n.Set("fr")
	defer i18n.Set("en")
	m := newModel(t, "true") // columns are created in French
	v := m.View()
	for _, want := range []string{"À faire", "En cours (AWF)", "Terminé", "entrée ↵", "1 carte", "workflow", "todo suivant"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q:\n%s", want, v)
		}
	}
}
