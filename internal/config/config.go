// Package config reads the global settings (~/.config/todo/config.yaml).
package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"todo/internal/board"
)

type Config struct {
	// AWF is the awf binary (default "awf").
	AWF string `yaml:"awf,omitempty"`
	// Transparent keeps the terminal background instead of painting one.
	Transparent bool `yaml:"transparent,omitempty"`
	// Columns is the layout given to new boards.
	Columns []board.Column `yaml:"columns,omitempty"`
	// Notifications controls the desktop notifications.
	Notifications Notifications `yaml:"notifications,omitempty"`
}

// Notifications selects the events that raise a notification. Absent keys mean true.
type Notifications struct {
	Enabled   *bool `yaml:"enabled,omitempty"`
	OnSuccess *bool `yaml:"on_success,omitempty"`
	OnFailure *bool `yaml:"on_failure,omitempty"`
	OnAsk     *bool `yaml:"on_ask,omitempty"`
	// Terminal is the fallback used without a D-Bus session: osc777 (default), osc9, bell or off.
	Terminal string `yaml:"terminal,omitempty"`
}

func on(b *bool) bool { return b == nil || *b }

// Active reports whether notifications are enabled at all.
func (n Notifications) Active() bool  { return on(n.Enabled) }
func (n Notifications) Success() bool { return n.Active() && on(n.OnSuccess) }
func (n Notifications) Failure() bool { return n.Active() && on(n.OnFailure) }
func (n Notifications) Ask() bool     { return n.Active() && on(n.OnAsk) }

// Path returns the config file location ($XDG_CONFIG_HOME/todo/config.yaml).
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "todo", "config.yaml")
}

// Load reads the config; when the file is missing it is created with defaults.
func Load() (*Config, error) { return LoadFrom(Path()) }

func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		c := &Config{AWF: "awf", Columns: board.DefaultColumns()}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			if out, err := yaml.Marshal(c); err == nil {
				_ = os.WriteFile(path, out, 0o644)
			}
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.AWF == "" {
		c.AWF = "awf"
	}
	if len(c.Columns) == 0 {
		c.Columns = board.DefaultColumns()
	}
	return &c, nil
}
