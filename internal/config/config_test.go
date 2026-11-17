package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "todo", "config.yaml")
	c, err := LoadFrom(p)
	if err != nil || c.AWF != "awf" || len(c.Columns) != 4 {
		t.Fatalf("%v %+v", err, c)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("config file not written")
	}
	c2, err := LoadFrom(p)
	if err != nil || !c2.Columns[1].AutoRun || c2.Columns[1].OnSuccess != "Done" {
		t.Fatalf("reload: %v %+v", err, c2.Columns)
	}
}
