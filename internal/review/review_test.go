package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) string {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")
	write(t, dir, "b.txt", "keep\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-qm", "init")
	return dir
}

func find(files []File, path string) *File {
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
}

func TestNotARepo(t *testing.T) {
	dir := t.TempDir()
	if rev, err := Snapshot(dir); rev != "" || err != nil {
		t.Fatalf("snapshot outside a repo: %q %v", rev, err)
	}
	if _, err := Diff(dir, ""); err != ErrNotRepo {
		t.Fatalf("want ErrNotRepo, got %v", err)
	}
}

func TestSnapshotCleanIsHead(t *testing.T) {
	dir := repo(t)
	rev, err := Snapshot(dir)
	if err != nil || rev == "" {
		t.Fatal(rev, err)
	}
	res, err := Diff(dir, rev)
	if err != nil || len(res.Files) != 0 || res.Fallback {
		t.Fatalf("clean tree: %+v %v", res, err)
	}
}

func TestDiffSinceSnapshotOnly(t *testing.T) {
	dir := repo(t)
	write(t, dir, "b.txt", "dirty before the run\n") // pre-existing change
	base, err := Snapshot(dir)
	if err != nil || base == "" {
		t.Fatal(base, err)
	}
	if out, _ := git(dir, 0, "status", "--porcelain"); out != " M b.txt\n" {
		t.Fatalf("snapshot must not touch the tree: %q", out)
	}
	// what the run does
	write(t, dir, "a.txt", "one\nTWO\nthree\nfour\n")
	write(t, dir, "new.go", "package x\n")
	write(t, dir, ".todo/board.yaml", "noise\n")
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}

	res, err := Diff(dir, base)
	if err != nil || res.Fallback {
		t.Fatal(res, err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("want a.txt, b.txt, new.go: %+v", res.Files)
	}
	a := find(res.Files, "a.txt")
	if a == nil || a.Adds != 2 || a.Dels != 1 || a.Status() != "M" {
		t.Fatalf("a.txt: %+v", a)
	}
	if b := find(res.Files, "b.txt"); b == nil || !b.Deleted || b.Status() != "D" {
		t.Fatalf("b.txt: %+v", b)
	}
	if n := find(res.Files, "new.go"); n == nil || !n.Added || n.Adds != 1 {
		t.Fatalf("new.go: %+v", n)
	}
	// line numbers
	for _, l := range a.Lines {
		if l.Kind == Add && l.Text == "four" && l.New != 4 {
			t.Fatalf("line number: %+v", l)
		}
		if l.Kind == Del && l.Old != 2 {
			t.Fatalf("line number: %+v", l)
		}
	}
}

func TestMissingBaseFallsBackToHead(t *testing.T) {
	dir := repo(t)
	write(t, dir, "a.txt", "changed\n")
	for _, base := range []string{"", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"} {
		res, err := Diff(dir, base)
		if err != nil || !res.Fallback || len(res.Files) != 1 {
			t.Fatalf("base %q: %+v %v", base, res, err)
		}
	}
}

func TestParseRenameAndBinary(t *testing.T) {
	out := "diff --git a/old.txt b/new.txt\nsimilarity index 100%\nrename from old.txt\nrename to new.txt\n" +
		"diff --git a/img.png b/img.png\nBinary files a/img.png and b/img.png differ\n"
	fs := Parse(out)
	if len(fs) != 2 || fs[0].Path != "new.txt" || fs[0].Status() != "R" || fs[0].OldPath != "old.txt" || !fs[1].Binary {
		t.Fatalf("%+v", fs)
	}
}
