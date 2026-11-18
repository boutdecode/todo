// Command todo is a kanban TUI whose cards can run AWF workflows.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"todo/internal/board"
	"todo/internal/config"
	"todo/internal/runner"
	"todo/internal/ui"
)

func main() {
	dir := flag.String("dir", ".", "projet AWF (le board est dans <dir>/.todo/board.yaml)")
	awf := flag.String("awf", "", "binaire awf (défaut : config, puis \"awf\")")
	transparent := flag.Bool("transparent", false, "garder le fond du terminal (pas de fond coloré)")
	flag.Parse()

	abs, err := filepath.Abs(*dir)
	if err != nil {
		fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Errorf("config %s : %w", config.Path(), err))
	}
	if *awf != "" {
		cfg.AWF = *awf
	}
	boards, err := board.LoadAll(abs, cfg.Columns)
	if err != nil {
		fatal(fmt.Errorf("board %s : %w", board.Dir(abs), err))
	}
	model := ui.New(abs, cfg.AWF, cfg, boards)
	model.SetTransparent(*transparent || cfg.Transparent)
	model.SetVersion(runner.Version(cfg.AWF))
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "todo:", err)
	os.Exit(1)
}
