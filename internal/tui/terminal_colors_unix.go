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
		if !waitForTTYData(fd, deadline) {
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
		// Reading to the end of the answer is what keeps a late report out of
		// the keyboard tcell is about to read.
		if hasDeviceAttributes(reply.String()) {
			return tcell.ColorDefault, tcell.ColorDefault, false
		}
	}
}

// waitForTTYData reports whether the terminal answered before the deadline. The
// set and the timeval are rebuilt per call: select leaves both undefined when
// it returns an error, EINTR included.
func waitForTTYData(fd int, deadline time.Time) bool {
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		timeval := unix.NsecToTimeval(int64(remaining))
		var readable unix.FdSet
		readable.Set(fd)
		ready, err := unix.Select(fd+1, &readable, nil, nil, &timeval)
		if err == unix.EINTR {
			continue
		}
		return err == nil && ready > 0
	}
}
