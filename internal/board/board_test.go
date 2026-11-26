package board

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndMoves(t *testing.T) {
	dir := t.TempDir()
	b, err := Load(dir, nil)
	if err != nil || len(b.Columns) != 4 {
		t.Fatalf("defaults: %v %d", err, len(b.Columns))
	}
	b.Add(0, Card{Title: "a", Workflow: "wf", Inputs: map[string]string{"k": "v"}})
	b.Add(0, Card{Title: "b"})
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	b2, err := Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := b2.Columns[0].Cards; len(got) != 2 || got[0].Inputs["k"] != "v" || got[0].ID == "" {
		t.Fatalf("round trip: %+v", got)
	}
	if i, ok := b2.Swap(0, 0, 1); !ok || i != 1 || b2.Columns[0].Cards[0].Title != "b" {
		t.Fatal("swap")
	}
	b2.Move(0, 0, 2)
	if len(b2.Columns[0].Cards) != 1 || len(b2.Columns[2].Cards) != 1 {
		t.Fatal("move")
	}
	id := b2.Columns[2].Cards[0].ID
	if ci, idx, ok := b2.Find(id); !ok || ci != 2 || idx != 0 {
		t.Fatal("find")
	}
}

func TestMultipleBoards(t *testing.T) {
	dir := t.TempDir()
	all, err := LoadAll(dir, nil)
	if err != nil || len(all) != 1 || all[0].Title() != "Board" {
		t.Fatalf("empty: %v %v", err, all)
	}
	all[0].Add(0, Card{Title: "x"})
	if err := all[0].Save(); err != nil {
		t.Fatal(err)
	}
	b := New(dir, "Sprint 2", nil)
	b2 := New(dir, "Sprint 2", nil)
	if filepath.Base(b.File) != "sprint-2.yaml" {
		t.Fatalf("file %s", b.File)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if b2 = New(dir, "Sprint 2", nil); filepath.Base(b2.File) != "sprint-2-2.yaml" {
		t.Fatalf("collision %s", b2.File)
	}
	all, err = LoadAll(dir, nil)
	if err != nil || len(all) != 2 || all[0].Title() != "Board" || all[1].Title() != "Sprint 2" || len(all[0].Columns[0].Cards) != 1 {
		t.Fatalf("reload: %v %+v", err, all)
	}
	if err := all[1].Delete(); err != nil {
		t.Fatal(err)
	}
	if all, _ = LoadAll(dir, nil); len(all) != 1 {
		t.Fatal("delete")
	}
}

func TestSaveAttachment(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveAttachment(dir, []byte("\x89PNG\r\n\x1a\nx"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) || filepath.Dir(path) != AttachmentsDir(dir) || filepath.Ext(path) != ".png" {
		t.Fatalf("unexpected path %q", path)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) != 9 {
		t.Fatalf("file not written: %v", err)
	}
}
