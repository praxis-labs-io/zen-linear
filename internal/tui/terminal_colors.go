package tui

import (
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/gdamore/tcell/v2"
)

const terminalQueryTimeout = 200 * time.Millisecond

// Device attributes go last: every terminal answers it, so its reply ends the read.
const terminalQuery = "\x1b]10;?\x1b\\\x1b]11;?\x1b\\\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c"

// The terminator is required, or a report still arriving matches short and scales a component wrong.
var oscColorReport = regexp.MustCompile(`\x1b\]([0-9]{1,2});rgba?:([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})(?:\x07|\x1b\\)`)

var deviceAttributesReport = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)

var kittyGraphicsReport = regexp.MustCompile(`\x1b_Gi=31(?:,[^;]*)?;OK\x1b\\`)

type terminalReply struct {
	background    tcell.Color
	foreground    tcell.Color
	colorsKnown   bool
	kittyGraphics bool
}

func hasDeviceAttributes(reply string) bool {
	return deviceAttributesReport.MatchString(reply)
}

func parseKittyGraphics(reply string) bool {
	return kittyGraphicsReport.MatchString(reply)
}

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

func scaleHexComponent(component string) int32 {
	value, err := strconv.ParseUint(component, 16, 32)
	if err != nil {
		return 0
	}
	full := float64(int64(1)<<(4*len(component)) - 1)
	return int32(math.Round(float64(value) / full * 255))
}
