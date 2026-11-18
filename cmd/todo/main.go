// Command todo is a kanban TUI whose cards can run AWF workflows.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"todo/internal/board"
	"todo/internal/config"
	"todo/internal/i18n"
	"todo/internal/runner"
	"todo/internal/ui"
)

func main() {
	// The language must be known before the flags are declared (their help is translated).
	lang := i18n.Detect()
	if v, ok := argValue(os.Args[1:], "lang"); ok {
		l, valid := i18n.Normalize(v)
		if !valid {
			i18n.Set(lang)
			fatal(fmt.Errorf(i18n.T("cli.badlang"), v))
		}
		lang = l
	}
	i18n.Set(lang)

	dir := flag.String("dir", ".", i18n.T("cli.dir"))
	awf := flag.String("awf", "", i18n.T("cli.awf"))
	transparent := flag.Bool("transparent", false, i18n.T("cli.transparent"))
	flag.String("lang", lang, i18n.T("cli.lang"))
	flag.Parse()

	abs, err := filepath.Abs(*dir)
	if err != nil {
		fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Errorf(i18n.T("cli.config"), config.Path(), err))
	}
	if *awf != "" {
		cfg.AWF = *awf
	}
	boards, err := board.LoadAll(abs, cfg.Columns)
	if err != nil {
		fatal(fmt.Errorf(i18n.T("cli.todos"), board.Dir(abs), err))
	}
	model := ui.New(abs, cfg.AWF, cfg, boards)
	model.SetTransparent(*transparent || cfg.Transparent)
	model.SetVersion(runner.Version(cfg.AWF))
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fatal(err)
	}
}

// argValue finds -name v, --name v, -name=v or --name=v in args.
func argValue(args []string, name string) (string, bool) {
	for i, a := range args {
		if a == "--" {
			break
		}
		a = strings.TrimLeft(a, "-")
		if a == name && strings.HasPrefix(args[i], "-") && i+1 < len(args) {
			return args[i+1], true
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok && strings.HasPrefix(args[i], "-") {
			return v, true
		}
	}
	return "", false
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "todo:", err)
	os.Exit(1)
}
