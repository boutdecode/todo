package board

import "testing"

func TestRoundTripAndMoves(t *testing.T) {
	dir := t.TempDir()
	b, err := Load(dir, nil)
	if err != nil || len(b.Columns) != 4 {
		t.Fatalf("defaults: %v %d", err, len(b.Columns))
	}
	b.Add(0, Card{Title: "a", Workflow: "wf", Inputs: map[string]string{"k": "v"}})
	b.Add(0, Card{Title: "b"})
	if err := b.Save(dir); err != nil {
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
