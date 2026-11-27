package localui

import (
	"strings"
	"unicode/utf8"
)

// plainTerminal strips terminal commands without buffering their (possibly
// unbounded) parameters. It is a transcript filter, not a screen emulator.
type plainTerminal struct {
	state       byte // 0=text, e=escape, i=escape intermediate, c=CSI, o=OSC, s=string, t=string escape
	stringState byte
	pending     [utf8.UTFMax]byte
	pendingLen  int
	cr          bool
}

func (p *plainTerminal) write(data []byte) string {
	var out strings.Builder
	for _, b := range data {
		if p.pendingLen == 0 && b < utf8.RuneSelf {
			p.writeRune(rune(b), &out)
			continue
		}
		p.pending[p.pendingLen] = b
		p.pendingLen++
		for p.pendingLen > 0 && utf8.FullRune(p.pending[:p.pendingLen]) {
			r, n := utf8.DecodeRune(p.pending[:p.pendingLen])
			copy(p.pending[:], p.pending[n:p.pendingLen])
			p.pendingLen -= n
			p.writeRune(r, &out)
		}
	}
	return out.String()
}

func (p *plainTerminal) writeRune(b rune, out *strings.Builder) {
	if b == 0x9c {
		p.state = 0
		return
	}
	switch p.state {
	case 'e':
		switch b {
		case '[':
			p.state = 'c'
		case ']':
			p.state = 'o'
		case 'P', 'X', '^', '_':
			p.state = 's'
		default:
			p.state = 0
			if b >= 0x20 && b <= 0x2f {
				p.state = 'i'
			}
		}
		return
	case 'i':
		if b >= 0x30 && b <= 0x7e {
			p.state = 0
		}
		return
	case 'c':
		if b >= 0x40 && b <= 0x7e {
			p.state = 0
		}
		return
	case 'o', 's':
		if b == 0x1b {
			p.stringState = p.state
			p.state = 't'
		} else if b == 7 && p.state == 'o' {
			p.state = 0
		}
		return
	case 't':
		if b == '\\' {
			p.state = 0
		} else if b == 7 && p.stringState == 'o' {
			p.state = 0
		} else if b != 0x1b {
			p.state = p.stringState
		}
		return
	}
	switch b {
	case 0x9b:
		p.state = 'c'
		return
	case 0x9d:
		p.state = 'o'
		return
	case 0x90, 0x98, 0x9e, 0x9f:
		p.state = 's'
		return
	}
	if b == 0x1b {
		p.state = 'e'
		return
	}
	if b == '\r' {
		out.WriteByte('\n')
		p.cr = true
		return
	}
	if b == '\n' {
		if !p.cr {
			out.WriteByte('\n')
		}
		p.cr = false
		return
	}
	if b < 0x20 && b != '\t' || b == 0x7f {
		return
	}
	p.cr = false
	if b < 0x80 || b > 0x9f {
		out.WriteRune(b)
	}
}
