package notify

import (
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestNilNotifierIsSafe(t *testing.T) {
	var n *Notifier
	n.Show("k", Success, "titre", "corps")
	n.Close()
}

func TestHints(t *testing.T) {
	cases := []struct {
		kind     Kind
		urgency  byte
		category string
	}{{Success, 1, "transfer.complete"}, {Failure, 2, "transfer.error"}, {Ask, 1, "im"}}
	for _, c := range cases {
		h := Hints(c.kind, "abc")
		if h["urgency"] != dbus.MakeVariant(c.urgency) || h["category"] != dbus.MakeVariant(c.category) {
			t.Errorf("kind %d: %v", c.kind, h)
		}
		if h["x-canonical-private-synchronous"] != dbus.MakeVariant("todo-abc") {
			t.Errorf("sync key: %v", h)
		}
	}
}

func TestSanitize(t *testing.T) {
	got := Sanitize("a\x1b]0;evil\x07b;c\n")
	if strings.ContainsAny(got, "\x1b\x07\n;") {
		t.Fatalf("not sanitized: %q", got)
	}
}

func TestTerminal(t *testing.T) {
	if got := Terminal(OSC777, false, "T", "B"); got != "\x1b]777;notify;T;B\x07" {
		t.Errorf("osc777: %q", got)
	}
	if got := Terminal(OSC9, false, "T", "B"); got != "\x1b]9;T: B\x07" {
		t.Errorf("osc9: %q", got)
	}
	if got := Terminal(Bell, true, "T", "B"); got != "\a" {
		t.Errorf("bell: %q", got)
	}
	if got := Terminal(OSC777, true, "T", "B"); got != "\x1bPtmux;\x1b\x1b]777;notify;T;B\x07\x1b\\" {
		t.Errorf("tmux: %q", got)
	}
	if Terminal(Off, false, "T", "B") != "" {
		t.Error("off must be silent")
	}
}

func TestEscapeBody(t *testing.T) {
	if got := escapeBody("<b>&"); got != "&lt;b&gt;&amp;" {
		t.Fatal(got)
	}
}
