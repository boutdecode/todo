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
}

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
