// Package clip reads an image from the system clipboard.
//
// Terminals only paste text, so the image is fetched with the platform's
// clipboard tool: wl-paste (Wayland), xclip (X11), pngpaste (macOS).
package clip

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// ErrNoImage is returned when the clipboard holds no image (or no tool is available).
var ErrNoImage = errors.New("no image in the clipboard")

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// IsPNG reports whether data starts with the PNG signature.
func IsPNG(data []byte) bool { return bytes.HasPrefix(data, pngMagic) }

// source is one way of getting a PNG out of the clipboard.
type source struct {
	name string
	args []string
	ok   func() bool
}

func sources() []source {
	if runtime.GOOS == "darwin" {
		return []source{{name: "pngpaste", args: []string{"-"}, ok: func() bool { return true }}}
	}
	return []source{
		{name: "wl-paste", args: []string{"--no-newline", "--type", "image/png"}, ok: func() bool { return os.Getenv("WAYLAND_DISPLAY") != "" }},
		{name: "xclip", args: []string{"-selection", "clipboard", "-t", "image/png", "-o"}, ok: func() bool { return os.Getenv("DISPLAY") != "" }},
	}
}

// Image returns the PNG held by the clipboard, or ErrNoImage.
func Image() ([]byte, error) {
	for _, s := range sources() {
		if !s.ok() {
			continue
		}
		path, err := exec.LookPath(s.name)
		if err != nil {
			continue
		}
		out, err := exec.Command(path, s.args...).Output()
		if err == nil && IsPNG(out) {
			return out, nil
		}
	}
	return nil, ErrNoImage
}
