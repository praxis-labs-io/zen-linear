//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package tui

func queryTerminal() terminalReply {
	return terminalReply{}
}
