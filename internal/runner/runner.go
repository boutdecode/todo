// Package runner runs a workflow through `awf run -o streaming` inside a PTY
// (adapted from awf-tui's awfrun) and turns its output into events.
package runner

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"gopkg.in/yaml.v3"
	"todo/internal/i18n"
)

// Event kinds.
const (
	KindStepStarted = "step.started"
	KindStepDone    = "step.done"
	KindOutput      = "output"
	KindAsk         = "ask"
	KindDone        = "done" // terminal; Status is completed|failed|cancelled
)

type Event struct {
	Kind   string
	Step   string
	Status string
	Stream string // "out" | "err"
	Line   string
	Error  string
	Prompt string
}

// Run is one workflow execution.
type Run struct {
	ID     string
	Events <-chan Event

	cmd       *exec.Cmd
	pty       *os.File
	mu        sync.Mutex
	cancelled bool
	convAsk   bool
	tail      []string
}

const (
	convPrompt = "> "
	// EOF ends a conversation step.
	EOF = "\x04"
)

var (
	reStarted = regexp.MustCompile(`^Workflow started: (\S+)`)
	reStep    = regexp.MustCompile(`^\s*→ (\S+)\s*$`)
	reStepOK  = regexp.MustCompile(`^\s*✓ (\S+): (\w+)\s*$`)
	reStepKO  = regexp.MustCompile(`^(\S+): (failed|cancelled|skipped)\s*$`)
	reFailed  = regexp.MustCompile(`^Workflow failed: (.*)$`)
)

// Args builds the `awf run` arguments (inputs sorted for stable output).
func Args(name string, inputs map[string]string) []string {
	args := []string{"run", name, "-o", "streaming"}
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--input", fmt.Sprintf("%s=%s", k, inputs[k]))
	}
	return args
}

func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	if ansi.Strip(strings.TrimLeft(string(data), "\r")) == convPrompt {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (r *Run) addTail(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tail = append(r.tail, line)
	if len(r.tail) > 200 {
		r.tail = r.tail[len(r.tail)-200:]
	}
}

func (r *Run) tailText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.TrimSpace(strings.Join(r.tail, "\n"))
}

func withOutput(msg, out string) string {
	if out == "" || strings.Contains(msg, out) {
		return msg
	}
	return msg + "\n" + out
}

// Start launches the workflow and waits for its execution id.
func Start(bin, dir, name string, inputs map[string]string) (*Run, error) {
	cmd := exec.Command(bin, Args(name, inputs)...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	// A PTY keeps stdout/stderr ordered and lets conversation steps start.
	pr, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 50, Cols: 1000})
	if err != nil {
		return nil, err
	}
	r := &Run{cmd: cmd, pty: pr}
	ch := make(chan Event, 256)
	r.Events = ch
	idc := make(chan string, 1)
	go r.pump(pr, ch, idc)

	select {
	case id := <-idc:
		if id == "" {
			return nil, errors.New(withOutput(i18n.T("err.noid"), r.tailText()))
		}
		return r, nil
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return nil, errors.New(withOutput(i18n.T("err.timeout"), r.tailText()))
	}
}

// Answer sends a line to a conversation step (EOF ends it).
func (r *Run) Answer(text string) error {
	r.mu.Lock()
	r.convAsk = false
	r.mu.Unlock()
	if text != EOF {
		text += "\n"
	}
	_, err := r.pty.WriteString(text)
	return err
}

// Cancel interrupts the workflow (Ctrl+C).
func (r *Run) Cancel() error {
	r.mu.Lock()
	r.cancelled = true
	r.mu.Unlock()
	return r.cmd.Process.Signal(os.Interrupt)
}

func (r *Run) pump(pr *os.File, ch chan<- Event, idc chan<- string) {
	defer close(ch)
	var (
		step, failMsg string
		sentID        bool
	)
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		sc.Split(splitLines)
		for sc.Scan() {
			line := strings.TrimRight(ansi.Strip(sc.Text()), "\r")
			if line == convPrompt {
				r.mu.Lock()
				r.convAsk = true
				r.mu.Unlock()
				ch <- Event{Kind: KindAsk, Step: step, Prompt: i18n.T("conv.prompt")}
				continue
			}
			r.addTail(line)
			switch {
			case reStarted.MatchString(line):
				if !sentID {
					sentID = true
					r.ID = reStarted.FindStringSubmatch(line)[1]
					idc <- r.ID
				}
			case reStep.MatchString(line):
				step = reStep.FindStringSubmatch(line)[1]
				ch <- Event{Kind: KindStepStarted, Step: step, Status: "running"}
			case reStepOK.MatchString(line):
				m := reStepOK.FindStringSubmatch(line)
				ch <- Event{Kind: KindStepDone, Step: m[1], Status: m[2]}
			case reStepKO.MatchString(line):
				m := reStepKO.FindStringSubmatch(line)
				ch <- Event{Kind: KindStepDone, Step: m[1], Status: m[2]}
			case strings.HasPrefix(line, "[OUT] "):
				ch <- Event{Kind: KindOutput, Step: step, Stream: "out", Line: line[6:]}
			case strings.HasPrefix(line, "[ERR] "):
				ch <- Event{Kind: KindOutput, Step: step, Stream: "err", Line: line[6:]}
			case reFailed.MatchString(line):
				failMsg = reFailed.FindStringSubmatch(line)[1]
			}
		}
	}()

	waitErr := r.cmd.Wait()
	select {
	case <-scanDone:
	case <-time.After(300 * time.Millisecond):
		pr.Close()
		<-scanDone
	}
	pr.Close()
	if !sentID {
		idc <- ""
	}

	r.mu.Lock()
	cancelled := r.cancelled
	r.mu.Unlock()
	switch {
	case cancelled:
		ch <- Event{Kind: KindDone, Status: "cancelled"}
	case failMsg != "" || waitErr != nil:
		if failMsg == "" {
			failMsg = waitErr.Error()
		}
		ch <- Event{Kind: KindDone, Status: "failed", Error: withOutput(failMsg, r.tailText())}
	default:
		ch <- Event{Kind: KindDone, Status: "completed"}
	}
}

// Workflows lists workflow names available for dir: the project's
// .awf/workflows, then the user's ($XDG_CONFIG_HOME/awf and ~/.awf).
func Workflows(dir string) []string {
	seen := map[string]bool{}
	var names []string
	for _, d := range searchDirs(dir) {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			n := e.Name()
			ext := filepath.Ext(n)
			if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
				continue
			}
			n = strings.TrimSuffix(n, ext)
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	return names
}

// Workflow describes a workflow available to cards.
type Workflow struct {
	Name   string
	Steps  []string
	Inputs []Input
}

// Input is an input declared in a workflow's `inputs:` section.
type Input struct {
	Name     string
	Required bool
	Default  string
}

// Summary is "a · b · c" for up to 3 steps, "N steps" beyond, "" when unknown.
func (w Workflow) Summary() string {
	switch {
	case len(w.Steps) == 0:
		return ""
	case len(w.Steps) <= 3:
		return strings.Join(w.Steps, " · ")
	}
	return i18n.T("steps.many", len(w.Steps))
}

func searchDirs(dir string) []string {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return []string{
		filepath.Join(dir, ".awf", "workflows"),
		filepath.Join(xdg, "awf", "workflows"),
		filepath.Join(home, ".awf", "workflows"),
	}
}

// WorkflowFile returns the YAML file of a workflow, or "".
func WorkflowFile(dir, name string) string {
	for _, d := range searchDirs(dir) {
		for _, ext := range []string{".yaml", ".yml"} {
			p := filepath.Join(d, name+ext)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// Steps lists the step names of a workflow in document order: the entries of
// `states:` except `initial` and terminal states.
func Steps(dir, name string) []string {
	p := WorkflowFile(dir, name)
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return parseSteps(data)
}

func parseSteps(data []byte) []string {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	var steps []string
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "states" {
			continue
		}
		st := root.Content[i+1]
		for j := 0; j+1 < len(st.Content); j += 2 {
			key, val := st.Content[j].Value, st.Content[j+1]
			if key == "initial" || val.Kind != yaml.MappingNode {
				continue
			}
			terminal := false
			for k := 0; k+1 < len(val.Content); k += 2 {
				if val.Content[k].Value == "type" && val.Content[k+1].Value == "terminal" {
					terminal = true
				}
			}
			if !terminal {
				steps = append(steps, key)
			}
		}
	}
	return steps
}

// Load reads a workflow's steps and declared inputs.
func Load(dir, name string) Workflow {
	w := Workflow{Name: name}
	p := WorkflowFile(dir, name)
	if p == "" {
		return w
	}
	if data, err := os.ReadFile(p); err == nil {
		w.Steps = parseSteps(data)
		w.Inputs = parseInputs(data)
	}
	return w
}

// parseInputs lists the entries of the top-level `inputs:` sequence.
func parseInputs(data []byte) []Input {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	var out []Input
	for i := 0; i+1 < len(root.Content); i += 2 {
		seq := root.Content[i+1]
		if root.Content[i].Value != "inputs" || seq.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range seq.Content {
			var in Input
			for k := 0; k+1 < len(item.Content); k += 2 {
				v := item.Content[k+1].Value
				switch item.Content[k].Value {
				case "name":
					in.Name = v
				case "required":
					in.Required = v == "true"
				case "default":
					in.Default = v
				}
			}
			if in.Name != "" {
				out = append(out, in)
			}
		}
	}
	return out
}

// ListWorkflows returns the workflows of dir with their steps.
func ListWorkflows(dir string) []Workflow {
	names := Workflows(dir)
	out := make([]Workflow, len(names))
	for i, n := range names {
		out[i] = Load(dir, n)
	}
	return out
}

// Version returns the output of `awf --version` trimmed to its last word (e.g. "0.11.2"), or "".
func Version(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return ""
	}
	return strings.TrimPrefix(f[len(f)-1], "v")
}

// Pause suspends the workflow process group (SIGSTOP).
func (r *Run) Pause() error { return syscall.Kill(-r.cmd.Process.Pid, syscall.SIGSTOP) }

// Resume continues a paused workflow (SIGCONT).
func (r *Run) Resume() error { return syscall.Kill(-r.cmd.Process.Pid, syscall.SIGCONT) }
