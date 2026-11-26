package clip

import "testing"

func TestIsPNG(t *testing.T) {
	if !IsPNG(append([]byte("\x89PNG\r\n\x1a\n"), 1, 2)) {
		t.Fatal("png signature must be recognised")
	}
	for _, d := range [][]byte{nil, []byte("hello"), []byte("\x89PN")} {
		if IsPNG(d) {
			t.Fatalf("%q is not a PNG", d)
		}
	}
}

func TestImageWithoutTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Image(); err != ErrNoImage {
		t.Fatalf("got %v, want ErrNoImage", err)
	}
}
