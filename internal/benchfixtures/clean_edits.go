package benchfixtures

import (
	"fmt"

	ts "github.com/odvcencio/gotreesitter"
)

// CleanEditGenerator proposes edits while tracking inserted trailing spaces.
type CleanEditGenerator struct{ inserted map[int]bool }

// CleanCandidate identifies a proposed edit.
type CleanCandidate struct {
	// Kind selects the edit operation.
	Kind string
	// At is the byte offset of the edit.
	At int
}

func cleanIdent(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// Candidates samples at most limit eligible positions evenly, with a seeded rotation.
func (g *CleanEditGenerator) Candidates(src []byte, kind string, limit int, seed uint64) []CleanCandidate {
	if limit <= 0 {
		return nil
	}
	positions := make([]int, 0)
	for i := 0; i < len(src); i++ {
		valid := false
		switch kind {
		case "space+":
			valid = src[i] == '\n'
		case "space-":
			valid = src[i] == ' ' && i+1 < len(src) && src[i+1] == '\n' && g.inserted[i]
		case "same":
			valid = src[i] >= '0' && src[i] <= '9' || i > 0 && i+1 < len(src) && cleanIdent(src[i-1]) && cleanIdent(src[i+1]) && (src[i] >= 'a' && src[i] <= 'z' || src[i] >= 'A' && src[i] <= 'Z')
		case "type":
			valid = i > 0 && i+1 < len(src) && cleanIdent(src[i-1]) && cleanIdent(src[i]) && cleanIdent(src[i+1])
		case "del":
			valid = i > 0 && i+2 < len(src) && cleanIdent(src[i-1]) && cleanIdent(src[i]) && cleanIdent(src[i+1]) && cleanIdent(src[i+2])
		}
		if valid {
			positions = append(positions, i)
		}
	}
	if len(positions) == 0 {
		return nil
	}
	if limit > len(positions) {
		limit = len(positions)
	}
	out := make([]CleanCandidate, 0, limit)
	offset := int(seed % uint64(len(positions)))
	for i := 0; i < limit; i++ {
		out = append(out, CleanCandidate{kind, positions[(offset+i*len(positions)/limit)%len(positions)]})
	}
	return out
}

// Apply constructs a proposal; callers record only accepted proposals with Accept.
func (g *CleanEditGenerator) Apply(src []byte, c CleanCandidate) (EditStep, error) {
	at := c.At
	if at < 0 || at >= len(src) {
		return EditStep{}, fmt.Errorf("position %d outside source", at)
	}
	oldEnd, newEnd := at, at
	var next []byte
	switch c.Kind {
	case "space+", "type":
		b := byte(' ')
		if c.Kind == "type" {
			b = 'x'
		}
		next = make([]byte, 0, len(src)+1)
		next = append(next, src[:at]...)
		next = append(next, b)
		next = append(next, src[at:]...)
		newEnd++
	case "space-", "del":
		if c.Kind == "space-" && (!g.inserted[at] || src[at] != ' ' || at+1 >= len(src) || src[at+1] != '\n') {
			return EditStep{}, fmt.Errorf("space was not inserted by generator")
		}
		next = make([]byte, 0, len(src)-1)
		next = append(next, src[:at]...)
		next = append(next, src[at+1:]...)
		oldEnd++
	case "same":
		next = append([]byte(nil), src...)
		b := src[at]
		switch {
		case b >= '0' && b <= '9':
			next[at] = '0' + (b-'0'+1)%10
		case b >= 'a' && b <= 'z':
			next[at] = 'a' + (b-'a'+1)%26
		case b >= 'A' && b <= 'Z':
			next[at] = 'A' + (b-'A'+1)%26
		default:
			return EditStep{}, fmt.Errorf("invalid replacement")
		}
		oldEnd++
		newEnd++
	default:
		return EditStep{}, fmt.Errorf("unknown kind %q", c.Kind)
	}
	return EditStep{Source: next, Edit: ts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd), StartPoint: sessionPoint(src, at), OldEndPoint: sessionPoint(src, oldEnd), NewEndPoint: sessionPoint(next, newEnd)}}, nil
}

// Accept shifts the tracked spaces after an accepted proposal.
func (g *CleanEditGenerator) Accept(c CleanCandidate) {
	next := make(map[int]bool, len(g.inserted)+1)
	for at := range g.inserted {
		switch c.Kind {
		case "space+", "type":
			if at >= c.At {
				at++
			}
		case "space-", "del":
			if at == c.At {
				continue
			}
			if at > c.At {
				at--
			}
		}
		next[at] = true
	}
	if c.Kind == "space+" {
		next[c.At] = true
	}
	g.inserted = next
}
