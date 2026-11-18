package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"todo/internal/notify"
	"todo/internal/runner"
)

type shown struct {
	key, summary, body string
	kind               notify.Kind
}

type fakeNotifier struct{ got []shown }

func (f *fakeNotifier) Show(key string, kind notify.Kind, summary, body string) {
	f.got = append(f.got, shown{key, summary, body, kind})
}

func notifModel(t *testing.T, script string) (*Model, *fakeNotifier) {
	m := newModel(t, script)
	f := &fakeNotifier{}
	m.SetNotifier(f)
	return m, f
}

func run(m *Model) {
	_, cmd := m.Update(key("enter"))
	drain(m, cmd)
}

func TestNotifySuccess(t *testing.T) {
	m, f := notifModel(t, `echo "Workflow started: r1"; echo ok`)
	run(m)
	if len(f.got) != 1 || f.got[0].kind != notify.Success || !strings.Contains(f.got[0].body, "Ma tâche") {
		t.Fatalf("%+v", f.got)
	}
}

func TestNotifyFailureHasError(t *testing.T) {
	m, f := notifModel(t, `echo "Workflow started: r2"; echo "Workflow failed: boom"; exit 1`)
	run(m)
	if len(f.got) != 1 || f.got[0].kind != notify.Failure || !strings.Contains(f.got[0].body, "boom") {
		t.Fatalf("%+v", f.got)
	}
}

func TestNotifyLaunchFailure(t *testing.T) {
	m, f := notifModel(t, "true")
	m.bin = "/nonexistent/awf"
	run(m)
	if len(f.got) != 1 || f.got[0].kind != notify.Failure {
		t.Fatalf("%+v", f.got)
	}
}

func TestNotifyCancelledIsSilent(t *testing.T) {
	m, f := notifModel(t, "true")
	m.runs["x"] = newRunState(nil, nil)
	m.onEvent(runEventMsg{card: "x", ev: runner.Event{Kind: runner.KindDone, Status: "cancelled"}})
	if len(f.got) != 0 {
		t.Fatalf("%+v", f.got)
	}
}

func TestNotifyAskOnceOnly(t *testing.T) {
	m, f := notifModel(t, "true")
	id := m.board.Columns[1].Cards[0].ID
	m.runs[id] = newRunState([]string{"chat"}, nil)
	ask := runEventMsg{card: id, ev: runner.Event{Kind: runner.KindAsk, Step: "chat", Prompt: "> "}}
	m.onEvent(ask)
	m.onEvent(ask)
	if len(f.got) != 1 || f.got[0].kind != notify.Ask || !strings.Contains(f.got[0].body, "chat") {
		t.Fatalf("%+v", f.got)
	}
	// answered, then asked again: a new notification
	m.runs[id].ask = ""
	m.onEvent(ask)
	if len(f.got) != 2 {
		t.Fatalf("%+v", f.got)
	}
}

func TestNotifyFocusRules(t *testing.T) {
	m, f := notifModel(t, `echo "Workflow started: r1"; echo ok`)
	m.Update(tea.FocusMsg{})
	run(m)
	if len(f.got) != 0 {
		t.Fatalf("focused window must not be notified: %+v", f.got)
	}
	m.Update(tea.BlurMsg{})
	m.notify("c", notify.Success, "b")
	if len(f.got) != 1 {
		t.Fatalf("blurred window must be notified: %+v", f.got)
	}
	m.Update(key("x")) // a key press proves the focus
	m.notify("c", notify.Success, "b")
	if len(f.got) != 1 {
		t.Fatalf("key press should restore focus: %+v", f.got)
	}
}

func TestNotifyWithoutFocusReporting(t *testing.T) {
	m, f := notifModel(t, `echo "Workflow started: r1"; echo ok`)
	m.Update(key("x"))
	run(m)
	if len(f.got) != 1 {
		t.Fatalf("no focus events: must notify: %+v", f.got)
	}
}

func TestNotifyConfigToggles(t *testing.T) {
	off := false
	m, f := notifModel(t, `echo "Workflow started: r1"; echo ok`)
	m.cfg.Notifications.OnSuccess = &off
	run(m)
	if len(f.got) != 0 {
		t.Fatalf("on_success: false: %+v", f.got)
	}
	m, f = notifModel(t, "true")
	m.cfg.Notifications.OnAsk = &off
	id := m.board.Columns[1].Cards[0].ID
	m.runs[id] = newRunState(nil, nil)
	m.onEvent(runEventMsg{card: id, ev: runner.Event{Kind: runner.KindAsk, Step: "s"}})
	if len(f.got) != 0 {
		t.Fatalf("on_ask: false: %+v", f.got)
	}
	m, f = notifModel(t, `echo "Workflow started: r1"; echo ok`)
	m.cfg.Notifications.Enabled = &off
	run(m)
	if len(f.got) != 0 {
		t.Fatalf("enabled: false: %+v", f.got)
	}
}

func TestNewModelHasNoNotifier(t *testing.T) {
	if m := newModel(t, "true"); m.nt != nil {
		t.Fatal("tests must never reach the desktop")
	}
}
