// Package notify shows desktop notifications (org.freedesktop.Notifications over D-Bus),
// falling back to terminal escape sequences (OSC 777 / OSC 9 / bell) when no session bus
// is reachable, e.g. over SSH. Every method is a no-op on a nil *Notifier.
package notify

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	dest  = "org.freedesktop.Notifications"
	path  = dbus.ObjectPath("/org/freedesktop/Notifications")
	iface = "org.freedesktop.Notifications"
	app   = "todo"
)

// Kind is the type of event a notification reports.
type Kind int

const (
	Success Kind = iota
	Failure
	Ask
)

// Terminal fallback modes.
const (
	OSC777 = "osc777"
	OSC9   = "osc9"
	Bell   = "bell"
	Off    = "off"
)

// Valid reports whether mode is a known terminal fallback mode ("" counts as default).
func Valid(mode string) bool {
	switch mode {
	case "", OSC777, OSC9, Bell, Off:
		return true
	}
	return false
}

// Notifier sends notifications, one replaceable slot per key (a card id).
type Notifier struct {
	conn     *dbus.Conn
	term     io.Writer // terminal fallback, nil when disabled
	mode     string
	mu       sync.Mutex
	ids      map[string]uint32
	closeTTY func()
}

// New connects to the session bus. If that fails and the terminal fallback is enabled,
// the notifier uses the terminal instead; it returns nil only when nothing is available.
func New(terminal string) (*Notifier, error) {
	if terminal == "" {
		terminal = OSC777
	}
	if !Valid(terminal) {
		return nil, fmt.Errorf("unknown terminal notification mode %q", terminal)
	}
	n := &Notifier{ids: map[string]uint32{}, mode: terminal}
	conn, err := dbus.ConnectSessionBus()
	if err == nil {
		n.conn = conn
		return n, nil
	}
	if terminal == Off {
		return nil, err
	}
	if tty, terr := os.OpenFile("/dev/tty", os.O_WRONLY, 0); terr == nil {
		n.term, n.closeTTY = tty, func() { _ = tty.Close() }
	} else {
		n.term = os.Stderr
	}
	return n, nil
}

// Close releases the connection and forgets the pending notification ids.
func (n *Notifier) Close() {
	if n == nil {
		return
	}
	if n.conn != nil {
		n.conn.Close()
	}
	if n.closeTTY != nil {
		n.closeTTY()
	}
	n.mu.Lock()
	n.ids = map[string]uint32{}
	n.mu.Unlock()
}

// Hints builds the D-Bus hints of a notification of the given kind for slot key.
func Hints(kind Kind, key string) map[string]dbus.Variant {
	urgency, category := byte(1), "transfer.complete"
	switch kind {
	case Failure:
		urgency, category = 2, "transfer.error"
	case Ask:
		category = "im"
	}
	return map[string]dbus.Variant{
		"urgency":                         dbus.MakeVariant(urgency),
		"category":                        dbus.MakeVariant(category),
		"desktop-entry":                   dbus.MakeVariant(app),
		"x-canonical-private-synchronous": dbus.MakeVariant("todo-" + key),
	}
}

func icon(kind Kind) string {
	switch kind {
	case Failure:
		return "dialog-error"
	case Ask:
		return "dialog-question"
	}
	return "dialog-information"
}

func timeout(kind Kind) int32 {
	if kind == Ask {
		return -1 // server default
	}
	return 5000
}

// Show displays (or replaces, for the same key) a notification. It never blocks.
func (n *Notifier) Show(key string, kind Kind, summary, body string) {
	if n == nil {
		return
	}
	if n.conn == nil {
		if n.term != nil {
			_, _ = io.WriteString(n.term, Terminal(n.mode, os.Getenv("TMUX") != "", summary, body))
		}
		return
	}
	n.mu.Lock()
	replaces := n.ids[key]
	n.mu.Unlock()
	call := n.conn.Object(dest, path).Go(iface+".Notify", 0, make(chan *dbus.Call, 1),
		app, replaces, icon(kind), summary, escapeBody(body), []string{}, Hints(kind, key), timeout(kind))
	go func() {
		<-call.Done
		var id uint32
		if call.Err == nil && call.Store(&id) == nil {
			n.mu.Lock()
			n.ids[key] = id
			n.mu.Unlock()
		}
	}()
}

// escapeBody escapes the markup some notification servers interpret.
func escapeBody(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// Sanitize removes control characters (which could inject escape sequences) and the
// ';' separator from text embedded in an OSC sequence.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ';':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		}
		return r
	}, s)
}

// Terminal returns the escape sequence notifying through the terminal; inTmux wraps it
// in tmux's passthrough envelope (needs `set -g allow-passthrough on`).
func Terminal(mode string, inTmux bool, summary, body string) string {
	summary, body = Sanitize(summary), Sanitize(body)
	var seq string
	switch mode {
	case OSC9:
		seq = "\x1b]9;" + summary + ": " + body + "\x07"
	case Bell:
		seq = "\a"
	case Off:
		return ""
	default:
		seq = "\x1b]777;notify;" + summary + ";" + body + "\x07"
	}
	if inTmux && mode != Bell {
		return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	return seq
}
