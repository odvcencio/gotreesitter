// Package luapattern translates the Lua pattern subset used by tree-sitter queries.
package luapattern

import (
	"regexp"
	"strings"
)

// Compile translates a Lua pattern into a Go regular expression.
func Compile(pattern string) (*regexp.Regexp, error) {
	var out strings.Builder
	inClass := false
	classContentStart := false

	writeLuaClass := func(ch byte, inClass bool, classContentStart bool) bool {
		inClassText := ""
		outsideText := ""
		if inClass {
			switch ch {
			case 'a':
				inClassText = "A-Za-z"
			case 'A':
				inClassText = "^A-Za-z"
			case 'c':
				inClassText = "[:cntrl:]"
			case 'C':
				inClassText = "^[:cntrl:]"
			case 'd':
				inClassText = "0-9"
			case 'D':
				inClassText = "^0-9"
			case 'l':
				inClassText = "a-z"
			case 'L':
				inClassText = "^a-z"
			case 'p':
				inClassText = "[:punct:]"
			case 'P':
				inClassText = "^[:punct:]"
			case 's':
				inClassText = "\\s"
			case 'S':
				inClassText = "^\\s"
			case 'u':
				inClassText = "A-Z"
			case 'U':
				inClassText = "^A-Z"
			case 'w':
				inClassText = "A-Za-z0-9"
			case 'W':
				inClassText = "^A-Za-z0-9"
			case 'x':
				inClassText = "A-Fa-f0-9"
			case 'X':
				inClassText = "^A-Fa-f0-9"
			case 'z':
				inClassText = "\\x00"
			case 'Z':
				inClassText = "^\\x00"
			default:
				return false
			}
			if strings.HasPrefix(inClassText, "^") && !classContentStart {
				return false
			}
			out.WriteString(inClassText)
			return true
		}
		switch ch {
		case 'a':
			outsideText = "[A-Za-z]"
		case 'A':
			outsideText = "[^A-Za-z]"
		case 'c':
			outsideText = "[[:cntrl:]]"
		case 'C':
			outsideText = "[^[:cntrl:]]"
		case 'd':
			outsideText = "[0-9]"
		case 'D':
			outsideText = "[^0-9]"
		case 'l':
			outsideText = "[a-z]"
		case 'L':
			outsideText = "[^a-z]"
		case 'p':
			outsideText = "[[:punct:]]"
		case 'P':
			outsideText = "[^[:punct:]]"
		case 's':
			outsideText = "\\s"
		case 'S':
			outsideText = "\\S"
		case 'u':
			outsideText = "[A-Z]"
		case 'U':
			outsideText = "[^A-Z]"
		case 'w':
			outsideText = "[A-Za-z0-9]"
		case 'W':
			outsideText = "[^A-Za-z0-9]"
		case 'x':
			outsideText = "[A-Fa-f0-9]"
		case 'X':
			outsideText = "[^A-Fa-f0-9]"
		case 'z':
			outsideText = "\\x00"
		case 'Z':
			outsideText = "[^\\x00]"
		default:
			return false
		}
		out.WriteString(outsideText)
		return true
	}

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '[':
			inClass = true
			classContentStart = true
			out.WriteByte(ch)
		case ']':
			inClass = false
			classContentStart = false
			out.WriteByte(ch)
		case '%':
			if i+1 >= len(pattern) {
				out.WriteString("%")
				continue
			}
			i++
			next := pattern[i]
			if writeLuaClass(next, inClass, classContentStart) {
				if inClass {
					classContentStart = false
				}
				continue
			}
			out.WriteString(regexp.QuoteMeta(string(next)))
			if inClass {
				classContentStart = false
			}
		case '-':
			if inClass {
				out.WriteByte(ch)
				classContentStart = false
				continue
			}
			out.WriteString("*?")
		default:
			out.WriteByte(ch)
			if inClass {
				classContentStart = false
			}
		}
	}

	return regexp.Compile(out.String())
}
