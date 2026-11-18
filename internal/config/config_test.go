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
	if err != nil || !c2.Columns[2].AutoRun || c2.Columns[2].OnSuccess != "Done" {
		t.Fatalf("reload: %v %+v", err, c2.Columns)
	}
}

func TestNotificationsDefaultsAndOverrides(t *testing.T) {
	var n Notifications
	if !n.Active() || !n.Success() || !n.Failure() || !n.Ask() {
		t.Fatal("absent keys must mean true")
	}
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("notifications:\n  on_success: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil || c.Notifications.Success() || !c.Notifications.Failure() {
		t.Fatalf("%v %+v", err, c.Notifications)
	}
	f := false
	c.Notifications.Enabled = &f
	if c.Notifications.Failure() || c.Notifications.Ask() {
		t.Fatal("enabled: false must disable everything")
	}
}
