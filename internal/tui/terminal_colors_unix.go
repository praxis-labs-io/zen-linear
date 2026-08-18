//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package tui

import (
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// queryTerminalColors asks the terminal for its own background and foreground,
// false when nothing answers. It goes raw, so it must run before tcell's own.
func queryTerminalColors() (background, foreground tcell.Color, ok bool) {
	termName := os.Getenv("TERM")
	if termName == "" || strings.HasPrefix(termName, "dumb") {
		return tcell.ColorDefault, tcell.ColorDefault, false
	}

	// /dev/tty rather than stdin, so a piped or redirected stream still reaches
	// the terminal the user is sitting at.
	fd, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return tcell.ColorDefault, tcell.ColorDefault, false
	}
	defer func() { _ = unix.Close(fd) }()

	if !term.IsTerminal(fd) {
		return tcell.ColorDefault, tcell.ColorDefault, false
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return tcell.ColorDefault, tcell.ColorDefault, false
	}
	defer func() { _ = term.Restore(fd, state) }()

	if _, err := unix.Write(fd, []byte(oscColorQuery)); err != nil {
		return tcell.ColorDefault, tcell.ColorDefault, false
	}

	deadline := time.Now().Add(terminalQueryTimeout)
	chunk := make([]byte, 256)
	var reply strings.Builder
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 || !waitForTTYData(fd, remaining) {
			return tcell.ColorDefault, tcell.ColorDefault, false
		}
		read, err := unix.Read(fd, chunk)
		if read > 0 {
			reply.Write(chunk[:read])
		}
		if err != nil {
			return tcell.ColorDefault, tcell.ColorDefault, false
		}
		if background, foreground, ok := parseTerminalColors(reply.String()); ok {
			return background, foreground, true
		}
	}
}

// waitForTTYData reports whether the terminal answered before the timeout.
func waitForTTYData(fd int, timeout time.Duration) bool {
	timeval := unix.NsecToTimeval(int64(timeout))
	var readable unix.FdSet
	readable.Set(fd)
	for {
		ready, err := unix.Select(fd+1, &readable, nil, nil, &timeval)
		if err == unix.EINTR {
			continue
		}
		return err == nil && ready > 0
	}
}
