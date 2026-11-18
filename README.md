# todo

Terminal kanban (Bubble Tea / Lip Gloss) whose cards can run [AWF](https://awf-project.ai) workflows.
Same family as `awf-tui`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/boutdecode/todo/main/install.sh | bash
```

Installs the latest release (Linux / macOS, amd64 / arm64) into `~/.local/bin` after verifying its checksum.
`VERSION=v0.1.0` pins a release, `INSTALL_DIR=/usr/local/bin` changes the target.

## Usage

```sh
make build
./todo -dir /path/to/awf-project      # default: current directory
./todo -awf /path/to/awf              # awf binary (default: config, then "awf")
./todo -transparent                   # keep the terminal background (or `transparent: true` in the config)
./todo -no-notify                     # disable desktop notifications
./todo -lang fr                       # interface language: en or fr (default: system language, else en)
```

The language is detected from `LC_ALL`, `LC_MESSAGES`, `LANGUAGE`, then `LANG`; `-lang` overrides it. It also sets
the names of the columns created by default (existing boards and configs keep theirs).

## Notifications

A notification is raised when a run **succeeds**, **fails**, or a `mode: conversation` step **waits for your answer**
(cancelled runs are silent). They are only sent while the terminal window is unfocused (this needs a terminal that
reports focus; otherwise they are always sent). There is one notification per card: the end of a run replaces its
pending question.

- **Linux desktop**: `org.freedesktop.Notifications` over D-Bus (no external command).
- **No session bus** (SSH, container): falls back to a terminal escape sequence, OSC 777 by default (Ghostty,
  WezTerm, foot, kitty…); set `notifications.terminal` to `osc9` (iTerm2, Windows Terminal, WezTerm), `bell` or `off`.
  Inside tmux, sequences go through passthrough: `set -g allow-passthrough on` (and `set -g focus-events on` for focus).
- **macOS**: no native notification; only the terminal fallback works, if your terminal supports it.

```yaml
notifications:
  enabled: true      # false = nothing (same as -no-notify)
  on_success: true
  on_failure: true
  on_ask: true
  terminal: osc777   # osc777 | osc9 | bell | off
```

## Storage

- **Project todos**: `<dir>/.todo/board.yaml` (first todo) and `<dir>/.todo/<name>.yaml` for the others (versionable, atomic writes). Each todo is a complete board; workflows of a todo keep running while another one is displayed.
- **Global config**: `~/.config/todo/config.yaml` (`$XDG_CONFIG_HOME`), created on first launch:
  the `awf` binary and the columns given to new boards.

```yaml
awf: awf
columns:
  - {name: Backlog, color: "#79c0ff"}
  - {name: To do, color: "#f2cc60"}
  - {name: In progress (AWF), color: "#ff7ac6", auto_run: true, on_success: Done, on_failure: Done}
  - {name: Done, color: "#7ee787"}
```

`auto_run` marks the **execution column**: `↵` moves the card there before starting its workflow, and a card whose
"Run automatically" box is checked starts as soon as it enters it. `on_success` / `on_failure`: column the card moves
to when the run ends (a failure shows as `✗` on the card). Existing boards keep their columns.

## Keys

| Key | Action |
|---|---|
| ← ↑ ↓ → | navigate |
| `↵` | run the workflow (the card moves to the execution column); on a running card: open its logs |
| `H` `L` | move the card (an "auto" card entering the execution column starts its workflow) |
| `K` `J` | reorder inside the column |
| `n` `e` `d` | new / edit / delete (confirm with `y`) |
| `l` | run logs (steps, timestamped output) |
| `r` | retry · `c` cancel |
| `tab` `shift+tab` | next / previous todo (tabs at the top) |
| `N` `R` `D` | new / rename / delete the todo (confirm with `y`, never the last one) |
| `/` · `esc` | search (title, workflow; non-matching cards are dimmed) / clear |
| `?` · `q` | help / quit (cancels running workflows) |

Logs view: `p` pause/resume (SIGSTOP/SIGCONT), `c` cancel, `↑↓` scroll, `f` follow the tail, `r` retry, `esc` back
(the run keeps going). Steps and the `step k/n · %` progress come from the `states:` of the workflow YAML.

Form: title, column, workflow (list filterable with `/`, step summary), optional `key=value, …` inputs,
auto-run checkbox. Badges: spinner running · ✓ · ✗ · ⊘ cancelled.

## Limits

- A run's output is only kept in memory (status, id and duration of the last run are saved in the board).
- `mode: conversation` steps are supported (`> ` prompt in the logs view); awf-tui's `@@AWF-ASK@@` protocol is not.
- The step total is inferred from the YAML: it can differ with loops or branches.
