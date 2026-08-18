package tui

import (
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/gdamore/tcell/v2"
)

// terminalQueryTimeout is how long the OSC query waits for an answer. It is
// paid once at launch by terminals that ignore the query, so it is short.
const terminalQueryTimeout = 200 * time.Millisecond

// oscColorQuery asks for the foreground (OSC 10) and background (OSC 11).
const oscColorQuery = "\x1b]10;?\x1b\\\x1b]11;?\x1b\\"

// oscColorReport matches a terminal's answer: an OSC code, then components of
// one to four hex digits each, BEL- or ST-terminated.
var oscColorReport = regexp.MustCompile(`\x1b\]([0-9]{1,2});rgba?:([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})`)

// parseTerminalColors reads the foreground and background out of whatever the
// terminal has sent so far. It reports false until both have arrived.
func parseTerminalColors(reply string) (background, foreground tcell.Color, ok bool) {
	var hasBackground, hasForeground bool
	for _, match := range oscColorReport.FindAllStringSubmatch(reply, -1) {
		color := tcell.NewRGBColor(
			scaleHexComponent(match[2]),
			scaleHexComponent(match[3]),
			scaleHexComponent(match[4]),
		)
		switch match[1] {
		case "10":
			foreground, hasForeground = color, true
		case "11":
			background, hasBackground = color, true
		}
	}
	return background, foreground, hasBackground && hasForeground
}

// scaleHexComponent widens a component of any width to 8 bits, since terminals
// answer in anything from rgb:1/2/3 to rgb:1c1c/1c1c/1c1c.
func scaleHexComponent(component string) int32 {
	value, err := strconv.ParseUint(component, 16, 32)
	if err != nil {
		return 0
	}
	full := float64(int64(1)<<(4*len(component)) - 1)
	return int32(math.Round(float64(value) / full * 255))
}
