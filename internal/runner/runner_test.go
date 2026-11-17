package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeAWF(t *testing.T, script string) string {
	p := filepath.Join(t.TempDir(), "awf")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func collect(t *testing.T, r *Run) []Event {
	var evs []Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-r.Events:
			if !ok {
				t.Fatal("channel closed without terminal event")
			}
			evs = append(evs, ev)
			if ev.Kind == KindDone {
				return evs
			}
		case <-timeout:
			t.Fatal("timeout")
		}
	}
}

func TestRunSuccess(t *testing.T) {
	bin := fakeAWF(t, `echo "Workflow started: abc123"; echo "  → build"; echo "[OUT] hello"; echo "  ✓ build: completed"; echo done`)
	r, err := Start(bin, t.TempDir(), "wf", map[string]string{"a": "1"})
	if err != nil || r.ID != "abc123" {
		t.Fatalf("start: %v %v", err, r)
	}
	evs := collect(t, r)
	last := evs[len(evs)-1]
	if last.Status != "completed" {
		t.Fatalf("last = %+v", last)
	}
	var out bool
	for _, e := range evs {
		if e.Kind == KindOutput && e.Line == "hello" {
			out = true
		}
	}
	if !out {
		t.Fatalf("no output event: %+v", evs)
	}
}

func TestRunFailureAndCancel(t *testing.T) {
	bin := fakeAWF(t, `echo "Workflow started: f1"; echo "Workflow failed: boom"; exit 1`)
	r, err := Start(bin, t.TempDir(), "wf", nil)
	if err != nil {
		t.Fatal(err)
	}
	if last := collect(t, r); last[len(last)-1].Status != "failed" {
		t.Fatalf("%+v", last)
	}

	bin = fakeAWF(t, `echo "Workflow started: c1"; exec sleep 30`)
	r, err = Start(bin, t.TempDir(), "wf", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Cancel()
	if last := collect(t, r); last[len(last)-1].Status != "cancelled" {
		t.Fatalf("%+v", last)
	}
}

func TestStartWithoutID(t *testing.T) {
	bin := fakeAWF(t, `echo nope; exit 2`)
	if _, err := Start(bin, t.TempDir(), "wf", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestArgs(t *testing.T) {
	got := Args("wf", map[string]string{"b": "2", "a": "1"})
	want := []string{"run", "wf", "-o", "streaming", "--input", "a=1", "--input", "b=2"}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
}

func TestParseSteps(t *testing.T) {
	y := `name: x
states:
  initial: a
  a:
    type: step
    on_success: b
  b:
    type: agent
    on_success: done
  done:
    type: terminal
  failed:
    type: terminal
`
	got := parseSteps([]byte(y))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%v", got)
	}
	if s := (Workflow{Steps: got}).Summary(); s != "a · b" {
		t.Fatal(s)
	}
	if s := (Workflow{Steps: []string{"1", "2", "3", "4"}}).Summary(); s != "4 étapes" {
		t.Fatal(s)
	}
}

func TestPauseResume(t *testing.T) {
	bin := fakeAWF(t, `echo "Workflow started: p1"; sleep 0.6; echo "[OUT] after"; echo ok`)
	r, err := Start(bin, t.TempDir(), "wf", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Pause(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(900 * time.Millisecond)
	select {
	case ev := <-r.Events:
		t.Fatalf("event while paused: %+v", ev)
	default:
	}
	if err := r.Resume(); err != nil {
		t.Fatal(err)
	}
	if last := collect(t, r); last[len(last)-1].Status != "completed" {
		t.Fatalf("%+v", last)
	}
}
