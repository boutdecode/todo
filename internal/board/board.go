// Package board holds the kanban model and its persistence (<dir>/.todo/board.yaml).
package board

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"todo/internal/i18n"

	"gopkg.in/yaml.v3"
)

// Run statuses stored on a card.
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

type LastRun struct {
	ID     string    `yaml:"id,omitempty"`
	Status string    `yaml:"status"`
	At     time.Time `yaml:"at"`
	Secs   int       `yaml:"secs,omitempty"` // duration of the run
	// BaseRev is the git revision of the tree when the run started (see review.Snapshot).
	BaseRev string `yaml:"base_rev,omitempty"`
}

type Card struct {
	ID          string            `yaml:"id"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description,omitempty"`
	Workflow    string            `yaml:"workflow,omitempty"`
	Inputs      map[string]string `yaml:"inputs,omitempty"`
	// AutoRun starts the workflow when the card enters an auto-run column.
	AutoRun bool     `yaml:"auto_run,omitempty"`
	LastRun *LastRun `yaml:"last_run,omitempty"`
}

type Column struct {
	Name      string `yaml:"name"`
	Color     string `yaml:"color,omitempty"`    // title color (hex), optional
	AutoRun   bool   `yaml:"auto_run,omitempty"` // execution column: workflows run while a card sits here
	OnSuccess string `yaml:"on_success,omitempty"`
	OnFailure string `yaml:"on_failure,omitempty"`
	Cards     []Card `yaml:"cards,omitempty"`
}

type Board struct {
	Name    string   `yaml:"name,omitempty"`
	File    string   `yaml:"-"`
	Columns []Column `yaml:"columns"`
}

// DefaultColumns is the layout used when neither the board nor the config defines one.
func DefaultColumns() []Column {
	return []Column{
		{Name: i18n.T("col.backlog"), Color: "#79c0ff"},
		{Name: i18n.T("col.todo"), Color: "#f2cc60"},
		{Name: i18n.T("col.doing"), Color: "#ff7ac6", AutoRun: true, OnSuccess: i18n.T("col.done"), OnFailure: i18n.T("col.done")},
		{Name: i18n.T("col.done"), Color: "#7ee787"},
	}
}

// DefaultName is the title of the first todo (<dir>/.todo/board.yaml).
const DefaultName = "Board"

// Dir returns the folder holding the todos of a project directory.
func Dir(dir string) string { return filepath.Join(dir, ".todo") }

// Path returns the file of the first todo of a project directory.
func Path(dir string) string { return filepath.Join(Dir(dir), "board.yaml") }

// Title returns the name shown for the board.
func (b *Board) Title() string {
	if b.Name == "" {
		return DefaultName
	}
	return b.Name
}

func read(file string, defaults []Column) (*Board, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		if len(defaults) == 0 {
			defaults = DefaultColumns()
		}
		return &Board{File: file, Columns: cloneColumns(defaults)}, nil
	}
	if err != nil {
		return nil, err
	}
	var b Board
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if len(b.Columns) == 0 {
		b.Columns = DefaultColumns()
	}
	b.File = file
	return &b, nil
}

func cloneColumns(cs []Column) []Column {
	out := make([]Column, len(cs))
	for i, c := range cs {
		c.Cards = nil
		out[i] = c
	}
	return out
}

// LoadAll reads every todo of dir: board.yaml first (an empty one when the
// file is missing), then the other .todo/*.yaml files sorted by name.
func LoadAll(dir string, defaults []Column) ([]*Board, error) {
	first, err := read(Path(dir), defaults)
	if err != nil {
		return nil, err
	}
	out := []*Board{first}
	files, _ := filepath.Glob(filepath.Join(Dir(dir), "*.yaml"))
	sort.Strings(files)
	for _, f := range files {
		if f == first.File {
			continue
		}
		b, err := read(f, defaults)
		if err != nil {
			return nil, err
		}
		if b.Name == "" {
			b.Name = strings.TrimSuffix(filepath.Base(f), ".yaml")
		}
		out = append(out, b)
	}
	return out, nil
}

// Load reads the first todo of dir; a missing file yields a board with the given columns.
func Load(dir string, defaults []Column) (*Board, error) { return read(Path(dir), defaults) }

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns a name into a file name stem.
func Slug(name string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "todo"
	}
	return s
}

// New creates an empty todo named name with a free file under dir/.todo.
func New(dir, name string, columns []Column) *Board {
	if len(columns) == 0 {
		columns = DefaultColumns()
	}
	stem := Slug(name)
	if stem == "board" {
		stem = "board-2"
	}
	file := filepath.Join(Dir(dir), stem+".yaml")
	for i := 2; ; i++ {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			break
		}
		file = filepath.Join(Dir(dir), stem+"-"+strconv.Itoa(i)+".yaml")
	}
	return &Board{Name: name, File: file, Columns: cloneColumns(columns)}
}

// Save writes the board atomically (temp file + rename).
func (b *Board) Save() error {
	data, err := yaml.Marshal(b)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.File), 0o755); err != nil {
		return err
	}
	tmp := b.File + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.File)
}

// Delete removes the board file.
func (b *Board) Delete() error {
	if err := os.Remove(b.File); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// NewID returns a short random card id.
func NewID() string {
	var raw [4]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// ColumnIndex returns the index of the column named name, or -1.
func (b *Board) ColumnIndex(name string) int {
	for i, c := range b.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// Find locates a card by id.
func (b *Board) Find(id string) (col, idx int, ok bool) {
	for ci, c := range b.Columns {
		for i, card := range c.Cards {
			if card.ID == id {
				return ci, i, true
			}
		}
	}
	return 0, 0, false
}

// RunColumn returns the index of the first auto-run column, or -1.
func (b *Board) RunColumn() int {
	for i, c := range b.Columns {
		if c.AutoRun {
			return i
		}
	}
	return -1
}

// Add appends a card at the end of column col.
func (b *Board) Add(col int, c Card) {
	if c.ID == "" {
		c.ID = NewID()
	}
	b.Columns[col].Cards = append(b.Columns[col].Cards, c)
}

// Remove deletes the card at (col, idx) and returns it.
func (b *Board) Remove(col, idx int) Card {
	cards := b.Columns[col].Cards
	c := cards[idx]
	b.Columns[col].Cards = append(cards[:idx:idx], cards[idx+1:]...)
	return c
}

// Move transfers a card to the end of column to and returns its new index.
func (b *Board) Move(col, idx, to int) int {
	c := b.Remove(col, idx)
	b.Columns[to].Cards = append(b.Columns[to].Cards, c)
	return len(b.Columns[to].Cards) - 1
}

// Swap exchanges the card at idx with the one at idx+d inside its column.
func (b *Board) Swap(col, idx, d int) (int, bool) {
	cards := b.Columns[col].Cards
	j := idx + d
	if j < 0 || j >= len(cards) {
		return idx, false
	}
	cards[idx], cards[j] = cards[j], cards[idx]
	return j, true
}
