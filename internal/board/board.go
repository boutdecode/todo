// Package board holds the kanban model and its persistence (<dir>/.todo/board.yaml).
package board

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

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
	Columns []Column `yaml:"columns"`
}

// DefaultColumns is the layout used when neither the board nor the config defines one.
func DefaultColumns() []Column {
	return []Column{
		{Name: "Backlog", Color: "#79c0ff"},
		{Name: "À faire", Color: "#f2cc60"},
		{Name: "En cours (AWF)", Color: "#ff7ac6", AutoRun: true, OnSuccess: "Terminé", OnFailure: "Terminé"},
		{Name: "Terminé", Color: "#7ee787"},
	}
}

// Path returns the board file of a project directory.
func Path(dir string) string { return filepath.Join(dir, ".todo", "board.yaml") }

// Load reads the board of dir; a missing file yields a board with the given columns.
func Load(dir string, defaults []Column) (*Board, error) {
	data, err := os.ReadFile(Path(dir))
	if errors.Is(err, os.ErrNotExist) {
		if len(defaults) == 0 {
			defaults = DefaultColumns()
		}
		return &Board{Columns: append([]Column(nil), defaults...)}, nil
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
	return &b, nil
}

// Save writes the board atomically (temp file + rename).
func (b *Board) Save(dir string) error {
	data, err := yaml.Marshal(b)
	if err != nil {
		return err
	}
	p := Path(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
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
