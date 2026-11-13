// Package lexexact extracts, from the text of a generated parser.c, the three
// facts per lex state that a grammar blob loses: the symbol of ACCEPT_TOKEN,
// the step that C takes at the end of input, and the step that C takes on an
// ill-formed byte (lookahead -1). It also encodes and decodes the record that
// holds them, and registers records by blob hash.
//
// The package reads text only. The contract:
//
// It reads parser.c files written by the tree-sitter generator. For such a file
// the record equals what the compiled lexer does; this was checked on the 240
// pinned grammar files and on 204 files regenerated with v0.27.1.
//
// For any other text, Parse refuses what it does not recognise: it accepts only
// a file made of the directives and top-level items that the generator writes,
// with a tree_sitter/parser.h from a fixed list. It does not promise to detect
// every hand-written file that uses features of C to change the meaning of text
// that has the generator's shape.
//
// Equality with the compiled lexer for one grammar is established by the probe,
// not by the reader alone: TestLexProbe in cgo_harness asks the compiled C
// lexer, for every lex state of ts_lex and ts_lex_keywords, for the accept
// symbol, the step at the end of input and the step on lookahead -1, and
// compares them with the record, and it compares the shipped blob's ranges with
// C on valid code points. A record must not be treated as checked without the
// probe's result for that file.
package lexexact

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

// Version is the record format version that this package writes and reads.
const Version = 1

// Magic starts every encoded record.
const Magic = "GTSLEXX"

// Tables is the record of one grammar.
type Tables struct {
	// Version is the record format version (1).
	Version uint8
	// BlobSHA256 is the hash of the grammar blob that the record was checked against.
	BlobSHA256 [32]byte
	// SourceSHA256 is the hash of the parser.c text that the record was built from.
	SourceSHA256 [32]byte
	// Main is the record of ts_lex; Keyword is that of ts_lex_keywords
	// (no states when the file has none).
	Main, Keyword Func
}

// Func is the record of one lex function. Every slice has one entry per lex
// state. A state without a case in the C function has zero entries.
type Func struct {
	// Accept is 0 when the state has no ACCEPT_TOKEN, otherwise symbol+1
	// (so 1 is ts_builtin_sym_end).
	Accept []uint32
	// OnEOF is the step at the end of input: 0 for no step, otherwise
	// (target+1)<<1 | skip.
	OnEOF []uint32
	// OnInvalid is the step on lookahead -1, in the same form as OnEOF.
	OnInvalid []uint32
	// Trans holds the ranges of a grammar whose blob is not C's table; nil
	// for a blob imported from parser.c.
	Trans [][]Range
	// Start is the start state per parse state when Trans is set; else nil.
	Start []uint32
}

// Range is one range of an own table: code points Lo..Hi (inclusive) lead to
// state Target; Skip is the SKIP flag.
type Range struct {
	Lo, Hi uint32
	Target uint32
	Skip   bool
}

// StepCode packs a step into the form of Func.OnEOF.
func StepCode(s Step, ok bool) uint32 {
	if !ok {
		return 0
	}
	v := uint32(s.Target+1) << 1
	if s.Skip {
		v |= 1
	}
	return v
}

// UnpackStep is the inverse of StepCode.
func UnpackStep(v uint32) (s Step, ok bool) {
	if v == 0 {
		return Step{}, false
	}
	return Step{Target: int(v>>1) - 1, Skip: v&1 != 0}, true
}

// Build computes the record of src and runs the blob check (CheckTransitions): the
// first matching range of the blob, for every state, must equal the step of
// the C text on every valid code point that matters. blobMain and blobKeyword
// are the blob's transitions per lex state (keyword may be empty when parser.c
// has no ts_lex_keywords). Build sets SourceSHA256 from parserC; the caller sets
// BlobSHA256. It rejects a record whose end-of-input steps form a cycle, on
// which C would not return.
func Build(src *Source, parserC string, blobMain, blobKeyword [][]Transition) (*Tables, error) {
	if err := src.Main.CheckTransitions(blobMain); err != nil {
		return nil, fmt.Errorf("ts_lex: blob differs from parser.c: %w", err)
	}
	if src.Keyword != nil {
		if err := src.Keyword.CheckTransitions(blobKeyword); err != nil {
			return nil, fmt.Errorf("ts_lex_keywords: blob differs from parser.c: %w", err)
		}
	} else if len(blobKeyword) != 0 {
		return nil, fmt.Errorf("ts_lex_keywords: the blob has %d states, parser.c has no such function", len(blobKeyword))
	}
	return BuildUnchecked(src, parserC)
}

// BuildUnchecked computes the record of src without the blob check, for a
// caller that compares the blob with C itself and wants to report every
// difference, and for tests. Build is the checked form.
func BuildUnchecked(src *Source, parserC string) (*Tables, error) {
	t := &Tables{Version: Version, SourceSHA256: sha256.Sum256([]byte(parserC))}
	var err error
	if t.Main, err = buildFunc(src.Main); err != nil {
		return nil, fmt.Errorf("ts_lex: %v", err)
	}
	if src.Keyword != nil {
		if t.Keyword, err = buildFunc(src.Keyword); err != nil {
			return nil, fmt.Errorf("ts_lex_keywords: %v", err)
		}
	}
	return t, nil
}

func buildFunc(l *Lexer) (Func, error) {
	n := l.NumStates()
	f := Func{Accept: make([]uint32, n), OnEOF: make([]uint32, n), OnInvalid: make([]uint32, n)}
	for i := 0; i < n; i++ {
		if sym, ok := l.Accept(i); ok {
			f.Accept[i] = uint32(sym) + 1
		}
		f.OnEOF[i] = StepCode(l.Step(i, true, 0))
		f.OnInvalid[i] = StepCode(l.Step(i, false, -1))
	}
	if l.own {
		f.Trans, f.Start = l.trans, l.start
	}
	if err := f.checkCycles(); err != nil {
		return f, err
	}
	return f, nil
}

// checkCycles rejects OnEOF steps that never end. At the end of input the
// walk does not move, so a cycle would run for ever in C.
func (f *Func) checkCycles() error {
	const (
		white = iota
		grey
		black
	)
	color := make([]uint8, len(f.OnEOF))
	for start := range f.OnEOF {
		var path []int
		s := start
		for {
			if color[s] == black {
				break
			}
			if color[s] == grey {
				return fmt.Errorf("end-of-input steps form a cycle at state %d", s)
			}
			color[s] = grey
			path = append(path, s)
			st, ok := UnpackStep(f.OnEOF[s])
			if !ok {
				break
			}
			s = st.Target
		}
		for _, p := range path {
			color[p] = black
		}
	}
	return nil
}

// ErrFormat is wrapped by every error that Decode and Validate return for a
// malformed record.
var ErrFormat = errors.New("lexexact: bad record")

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrFormat, fmt.Sprintf(format, a...))
}

// Validate checks the record against the number of lex states of the blob
// (len(LexStates) and len(KeywordLexStates)), and everything that Decode checks.
// A record with own ranges has its own state count and is exempt from the
// state-count check.
func (t *Tables) Validate(mainStates, keywordStates int) error {
	if err := t.validate(); err != nil {
		return err
	}
	for _, c := range []struct {
		name string
		f    *Func
		n    int
	}{{"main", &t.Main, mainStates}, {"keyword", &t.Keyword, keywordStates}} {
		if c.f.Trans == nil && len(c.f.Accept) != c.n {
			return bad("%s function has %d states, the blob has %d", c.name, len(c.f.Accept), c.n)
		}
	}
	return nil
}

// validate is the one validation behind Validate, Encode and Decode.
func (t *Tables) validate() error {
	if t.Version != Version {
		return bad("version %d", t.Version)
	}
	if err := t.Main.validate("main"); err != nil {
		return err
	}
	return t.Keyword.validate("keyword")
}

func (f *Func) validate(name string) error {
	n := len(f.Accept)
	// The domain that Parse and EmitC can represent: state numbers and symbols fit uint16_t.
	if n > maxState+1 {
		return bad("%s function: %d states, at most %d", name, n, maxState+1)
	}
	for i, a := range f.Accept {
		if a > maxSymbol+1 {
			return bad("%s function: state %d has accept code %d, at most %d", name, i, a, maxSymbol+1)
		}
	}
	if len(f.OnEOF) != n || len(f.OnInvalid) != n {
		return bad("%s function: slices of different length", name)
	}
	for _, steps := range [][]uint32{f.OnEOF, f.OnInvalid} {
		for i, v := range steps {
			// 0 is "no step"; otherwise v>>1 is target+1, so it must be in 1..n.
			if v != 0 && (uint64(v>>1) == 0 || uint64(v>>1) > uint64(n)) {
				return bad("%s function: state %d has step code %d, outside the %d states", name, i, v, n)
			}
		}
	}
	if f.Trans == nil && f.Start != nil {
		return bad("%s function: start states without ranges", name)
	}
	if f.Trans != nil {
		if len(f.Trans) != n || len(f.Start) == 0 {
			return bad("%s function: own ranges do not match the state count", name)
		}
		for _, s := range f.Start {
			if uint64(s) >= uint64(n) {
				return bad("%s function: start state %d of %d", name, s, n)
			}
		}
		for i, rs := range f.Trans {
			for _, r := range rs {
				if r.Lo > r.Hi || r.Hi > MaxCodePoint || uint64(r.Target) >= uint64(n) {
					return bad("%s function: state %d has a bad range %d..%d to state %d", name, i, r.Lo, r.Hi, r.Target)
				}
			}
		}
	}
	if err := f.checkCycles(); err != nil {
		return bad("%s function: %v", name, err)
	}
	return nil
}
