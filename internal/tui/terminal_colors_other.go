//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package tui

import "github.com/gdamore/tcell/v2"

// queryTerminalColors has no answer off unix: the query needs a controlling tty
// to read from and a raw mode to read it in.
func queryTerminalColors() (background, foreground tcell.Color, ok bool) {
	return tcell.ColorDefault, tcell.ColorDefault, false
}
