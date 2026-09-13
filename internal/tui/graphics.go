package tui

import (
	"encoding/base64"
	"fmt"
	"io"
)

type graphicsProtocol interface {
	Transmit(w io.Writer, id uint32, data []byte) error
	Place(w io.Writer, id, placement uint32, cols, rows int) error
	Delete(w io.Writer, id, placement uint32) error
}

// The kitty protocol caps a payload at 4096 base64 characters.
const kittyChunk = 4096

type kittyGraphics struct{}

// Every command carries q=2: a reply would arrive on the tty tcell reads and be delivered as keys.
func (kittyGraphics) Transmit(w io.Writer, id uint32, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("kitty: image %d is empty", id)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	for first := true; len(encoded) > 0; first = false {
		chunk := encoded
		if len(chunk) > kittyChunk {
			chunk = chunk[:kittyChunk]
		}
		encoded = encoded[len(chunk):]

		more := 0
		if len(encoded) > 0 {
			more = 1
		}
		control := fmt.Sprintf("m=%d", more)
		if first {
			control = fmt.Sprintf("a=t,f=100,t=d,i=%d,q=2,m=%d", id, more)
		}

		if _, err := fmt.Fprintf(w, "\x1b_G%s;%s\x1b\\", control, chunk); err != nil {
			return fmt.Errorf("kitty: transmit image %d: %w", id, err)
		}
	}
	return nil
}

func (kittyGraphics) Place(w io.Writer, id, placement uint32, cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("kitty: image %d has no room: %dx%d", id, cols, rows)
	}
	if _, err := fmt.Fprintf(w, "\x1b_Ga=p,i=%d,p=%d,c=%d,r=%d,C=1,q=2\x1b\\", id, placement, cols, rows); err != nil {
		return fmt.Errorf("kitty: place image %d: %w", id, err)
	}
	return nil
}

// d=i drops the placement and keeps the bytes, so the next frame places the image without re-sending it.
func (kittyGraphics) Delete(w io.Writer, id, placement uint32) error {
	if _, err := fmt.Fprintf(w, "\x1b_Ga=d,d=i,i=%d,p=%d,q=2\x1b\\", id, placement); err != nil {
		return fmt.Errorf("kitty: delete image %d: %w", id, err)
	}
	return nil
}
