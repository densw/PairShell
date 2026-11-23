package main

import (
	"io"
	"sync"
	"unicode/utf8"
)

type displayState uint8

const (
	displayText displayState = iota
	displayEscape
	displayCharset
	displayCSI
	displayControlString
	displayStringEscape
)

// terminalDisplay reduces remote output to display controls and text before it
// reaches a local renderer. Keep one instance per SSH output stream.
type terminalDisplay struct {
	output      io.Writer
	writeMu     *sync.Mutex
	state       displayState
	osc         bool
	csi         [128]byte
	csiLength   int
	csiOverflow bool
	charset     byte
	utf8Pending [utf8.UTFMax]byte
	utf8Length  int
}

// Both file descriptors can point at the same terminal. Keep complete output
// chunks serialized without letting a partial sequence move between streams.
func terminalDisplays(stdout, stderr io.Writer) (io.Writer, io.Writer) {
	mu := new(sync.Mutex)
	return &terminalDisplay{output: stdout, writeMu: mu}, &terminalDisplay{output: stderr, writeMu: mu}
}

func (d *terminalDisplay) Write(p []byte) (int, error) {
	data := p
	if d.utf8Length != 0 {
		data = append(d.utf8Pending[:d.utf8Length:d.utf8Length], p...)
		d.utf8Length = 0
	}
	visible := make([]byte, 0, len(data))
	for len(data) != 0 {
		if !utf8.FullRune(data) {
			d.utf8Length = copy(d.utf8Pending[:], data)
			break
		}
		r, size := utf8.DecodeRune(data)
		encoded := data[:size]
		if r == utf8.RuneError && size == 1 {
			if data[0] >= 0x80 && data[0] <= 0x9f {
				r = rune(data[0])
			} else {
				encoded = []byte("\uFFFD")
			}
		}
		visible = d.decode(visible, r, encoded)
		data = data[size:]
	}
	if len(visible) != 0 {
		if d.writeMu != nil {
			d.writeMu.Lock()
			defer d.writeMu.Unlock()
		}
		n, err := d.output.Write(visible)
		if err != nil {
			return 0, err
		}
		if n != len(visible) {
			return 0, io.ErrShortWrite
		}
	}
	return len(p), nil
}

func (d *terminalDisplay) beginCSI() {
	d.state = displayCSI
	d.csiLength = 0
	d.csiOverflow = false
}

func (d *terminalDisplay) decode(out []byte, r rune, encoded []byte) []byte {
	if d.state == displayControlString || d.state == displayStringEscape {
		switch {
		case r == 0x9c || r == 0x18 || r == 0x1a || (d.osc && r == '\a') || (d.state == displayStringEscape && r == '\\'):
			d.state = displayText
		case r == 0x1b:
			d.state = displayStringEscape
		default:
			d.state = displayControlString
		}
		return out
	}
	if r == 0x1b {
		d.state = displayEscape
		return out
	}
	if r >= 0x80 && r <= 0x9f {
		d.state = displayText
		switch r {
		case 0x90, 0x98, 0x9d, 0x9e, 0x9f:
			d.state, d.osc = displayControlString, r == 0x9d
		case 0x9b:
			d.beginCSI()
		case 0x84, 0x85, 0x88, 0x8d, 0x8e, 0x8f:
			return append(out, 0x1b, byte(r-0x40))
		}
		return out
	}
	switch d.state {
	case displayEscape:
		d.state = displayText
		switch r {
		case ']', 'P', '_', '^', 'X':
			d.state, d.osc = displayControlString, r == ']'
		case '[':
			d.beginCSI()
		case '(', ')', '*', '+':
			d.state, d.charset = displayCharset, byte(r)
		case '7', '8', '=', '>', 'D', 'E', 'H', 'M', 'N', 'O', 'c':
			return append(out, 0x1b, byte(r))
		}
	case displayCharset:
		d.state = displayText
		switch r {
		case '0', 'A', 'B':
			return append(out, 0x1b, d.charset, byte(r))
		}
	case displayCSI:
		switch {
		case r >= 0x20 && r <= 0x3f:
			if d.csiLength == len(d.csi) {
				d.csiOverflow = true
			} else {
				d.csi[d.csiLength] = byte(r)
				d.csiLength++
			}
		case r >= 0x40 && r <= 0x7e:
			d.state = displayText
			// Window-management, printer and terminal attribute requests
			// have local effects outside the remote terminal's screen.
			if !d.csiOverflow && r != 't' && r != 'i' && r != 'c' && r != 'x' {
				out = append(out, 0x1b, '[')
				out = append(out, d.csi[:d.csiLength]...)
				return append(out, byte(r))
			}
		default:
			d.state = displayText
		}
	case displayText:
		if r >= 0x20 && r != 0x7f || r >= '\b' && r <= 0x0f {
			return append(out, encoded...)
		}
	}
	return out
}
