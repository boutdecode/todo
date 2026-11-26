// Package review computes what a workflow run changed in a git working tree, for the
// code review view. It only reads the repository: the snapshot commit it creates has no
// reference and the working tree is never touched.
package review

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrNotRepo is returned when the directory is not inside a git work tree.
var ErrNotRepo = errors.New("not a git repository")

const (
	gitTimeout = 30 * time.Second
	maxLines   = 5000 // per file; the rest is dropped (File.Truncated)
	stateDir   = ".todo"
)

// git runs a git command in dir. okExit lists non-zero exit codes that are not errors
// (git diff --no-index exits 1 when the files differ).
func git(dir string, okExit int, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotepath=false"}, args...)...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && okExit != 0 && ee.ExitCode() == okExit {
			return out.String(), nil
		}
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return out.String(), nil
}

// Root returns the top level of the work tree containing dir.
func Root(dir string) (string, error) {
	out, err := git(dir, 0, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotRepo
	}
	return strings.TrimSpace(out), nil
}

// Snapshot returns a revision describing the tree as it is now: a dangling commit made
// by `git stash create` when tracked files are modified, HEAD otherwise. It returns ""
// (and no error) when dir is not a repository or has no commit yet.
func Snapshot(dir string) (string, error) {
	root, err := Root(dir)
	if err != nil {
		return "", nil
	}
	head, err := git(root, 0, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return "", nil // no commit yet
	}
	out, err := git(root, 0, "stash", "create")
	if err != nil {
		return "", err
	}
	if rev := strings.TrimSpace(out); rev != "" {
		return rev, nil
	}
	return strings.TrimSpace(head), nil
}

// Result is the set of changes between a base revision and the working tree.
type Result struct {
	Base     string // revision actually compared
	Fallback bool   // the requested base was missing: HEAD was used instead
	Files    []File
}

// Diff compares base with the working tree, untracked files included. An empty or
// vanished base falls back to HEAD (Result.Fallback).
func Diff(dir, base string) (*Result, error) {
	root, err := Root(dir)
	if err != nil {
		return nil, err
	}
	res := &Result{Base: base}
	if base != "" {
		if _, err := git(root, 0, "cat-file", "-e", base+"^{commit}"); err != nil {
			base = ""
		}
	}
	if base == "" {
		res.Fallback = true
		head, err := git(root, 0, "rev-parse", "--verify", "-q", "HEAD")
		if err != nil {
			return nil, errors.New("no commit to compare with")
		}
		base = strings.TrimSpace(head)
	}
	res.Base = base

	args := []string{"diff", "--no-color", "--no-ext-diff", base, "--", ".", ":(exclude)" + stateDir}
	out, err := git(root, 0, args...)
	if err != nil {
		return nil, err
	}
	res.Files = Parse(out)

	list, err := git(root, 0, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(list, "\x00") {
		if p == "" || p == stateDir || strings.HasPrefix(p, stateDir+"/") {
			continue
		}
		d, err := git(root, 1, "diff", "--no-color", "--no-ext-diff", "--no-index", "--", "/dev/null", p)
		if err != nil {
			continue // unreadable file (broken symlink…): skip it
		}
		res.Files = append(res.Files, Parse(d)...)
	}
	return res, nil
}

// Kind classifies a diff line.
type Kind int

const (
	Context Kind = iota
	Add
	Del
	Hunk // "@@ -a,b +c,d @@" header
	Note // "\ No newline at end of file"
)

// Line is one line of a file diff. Old and New are line numbers (0 when not applicable).
type Line struct {
	Kind     Kind
	Text     string // without the leading +/-/space
	Old, New int
}

// File is the diff of one path.
type File struct {
	Path       string
	OldPath    string // set for renames
	Added      bool
	Deleted    bool
	Binary     bool
	Truncated  bool
	Adds, Dels int
	Lines      []Line
}

// Status returns a one-letter status: A, D, R or M.
func (f File) Status() string {
	switch {
	case f.Added:
		return "A"
	case f.Deleted:
		return "D"
	case f.OldPath != "":
		return "R"
	}
	return "M"
}

// Parse reads the output of git diff into files.
func Parse(out string) []File {
	var files []File
	var cur *File
	var o, n int
	inHunk := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "diff --git ") {
			files = append(files, File{Path: pathFromHeader(l)})
			cur = &files[len(files)-1]
			inHunk = false
			continue
		}
		if cur == nil {
			continue
		}
		if !inHunk {
			switch {
			case strings.HasPrefix(l, "new file mode"):
				cur.Added = true
			case strings.HasPrefix(l, "deleted file mode"):
				cur.Deleted = true
			case strings.HasPrefix(l, "rename from "):
				cur.OldPath = strings.TrimPrefix(l, "rename from ")
			case strings.HasPrefix(l, "rename to "):
				cur.Path = strings.TrimPrefix(l, "rename to ")
			case strings.HasPrefix(l, "Binary files "), l == "GIT binary patch":
				cur.Binary = true
			case strings.HasPrefix(l, "@@"):
				inHunk = true
			}
			if !inHunk {
				continue
			}
		}
		if len(cur.Lines) >= maxLines {
			cur.Truncated = true
			continue
		}
		switch {
		case strings.HasPrefix(l, "@@"):
			o, n = hunkStart(l)
			cur.Lines = append(cur.Lines, Line{Kind: Hunk, Text: l})
		case strings.HasPrefix(l, "+"):
			cur.Lines = append(cur.Lines, Line{Kind: Add, Text: l[1:], New: n})
			cur.Adds++
			n++
		case strings.HasPrefix(l, "-"):
			cur.Lines = append(cur.Lines, Line{Kind: Del, Text: l[1:], Old: o})
			cur.Dels++
			o++
		case strings.HasPrefix(l, "\\"):
			cur.Lines = append(cur.Lines, Line{Kind: Note, Text: strings.TrimSpace(l[1:])})
		case strings.HasPrefix(l, " "):
			cur.Lines = append(cur.Lines, Line{Kind: Context, Text: l[1:], Old: o, New: n})
			o++
			n++
		}
	}
	return files
}

// pathFromHeader extracts the path from "diff --git a/x b/x" (best effort: renames and
// paths with " b/" are corrected later from the extended headers).
func pathFromHeader(l string) string {
	rest := strings.TrimPrefix(l, "diff --git ")
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return rest
}

// hunkStart reads the old and new start lines of "@@ -a,b +c,d @@".
func hunkStart(l string) (o, n int) {
	f := strings.Fields(l)
	if len(f) < 3 {
		return 0, 0
	}
	num := func(s string) int {
		s = strings.TrimLeft(s, "-+")
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[:i]
		}
		v, _ := strconv.Atoi(s)
		return v
	}
	return num(f[1]), num(f[2])
}
