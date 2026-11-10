package ccore

// Trace: one guarded call per C LOG line (C/parser.c LOG, ts_parser__log).
// Every call site reads `if p.logf != nil { p.log(...) }`, so no argument is
// formatted or boxed while the logger is off.

import (
	"fmt"
	"strings"
)

// SetLogger sets the receiver of C's parse log lines; nil turns logging off.
func (p *Parser) SetLogger(f func(string)) { p.logf = f }

func (p *Parser) log(format string, args ...any) {
	if p.logf != nil {
		p.logf(fmt.Sprintf(format, args...))
	}
}

// escape is C/parser.c's symbol-name escaping for log lines.
func escape(name string) string {
	if !strings.ContainsAny(name, "\t\n\v\f\r\\") {
		return name
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\v':
			b.WriteString(`\v`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteByte(name[i])
		}
	}
	return b.String()
}
