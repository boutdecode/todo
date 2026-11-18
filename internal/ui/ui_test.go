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
	b, _ := board.Load(dir, nil)
	b.Add(1, board.Card{Title: "Ma tâche", Workflow: "wf", AutoRun: true}) // "À faire"
	cfg := &config.Config{AWF: bin, Columns: board.DefaultColumns()}
	m := New(dir, bin, cfg, b)
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
		t.Fatalf("card should be in Terminé: %+v", m.board.Columns)
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
	if v := m.View(); !strings.Contains(v, "✗") || !strings.Contains(v, "r pour relancer") {
		t.Fatalf("failure not shown:\n%s", v)
	}
}

func TestFormCreatesCard(t *testing.T) {
	m := newModel(t, "true")
	m.Update(key("n"))
	for _, r := range "Nouvelle" {
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
		if v := m.View(); !strings.Contains(v, "BACKLOG") && !strings.Contains(v, "Backlog") || !strings.Contains(v, "En cours") || !strings.Contains(v, "entrée ↵") {
			t.Fatalf("missing board content:\n%s", v)
		}
		m.Update(key("n"))
		check("form")
		if !strings.Contains(m.View(), "Nouvelle tâche") {
			t.Fatal("form title")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m.Update(key("/"))
		for _, r := range "ma" {
			m.Update(key(string(r)))
		}
		check("filter")
		if !strings.Contains(m.View(), "1 résultat") {
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
	if !strings.Contains(v, "Étapes") || !strings.Contains(v, "Logs (stream)") || !strings.Contains(v, "retour board") {
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
