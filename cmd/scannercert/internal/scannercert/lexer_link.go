package scannercert

import (
	"fmt"
	"reflect"
	"sync"
	_ "unsafe" // for go:linkname

	gts "github.com/odvcencio/gotreesitter"
)

// The root package exports no constructor for ExternalLexer and no reader for
// the token a scan produced. The scanner tests under grammars/runtime reach
// both through go:linkname, and so does this package. The linker does not
// compare signatures across a linkname, so lexer_link_test.go exercises both
// functions and the fields read below.
//
// Only root tests and this package call newExternalLexer, so a dead-code
// cleanup in the root could remove it. That would break the link of
// cmd/scannercert, and CI compiles every command, so the build would fail
// there. Keep newExternalLexer, or give this package another way to build a
// lexer.

// newLexer builds the lexer the parser builds before a scan.
//
//go:linkname newLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newLexer(source []byte, pos int, row, column uint32) *gts.ExternalLexer

// lexerToken returns the token a scan left in the lexer and whether the
// scanner set a result symbol. It has no side effects.
//
//go:linkname lexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func lexerToken(*gts.ExternalLexer) (gts.Token, bool)

// lexerFields holds the positions of the ExternalLexer fields that describe
// where a scan began.
type lexerFields struct {
	source, pos, point int
	row, column        int
}

var (
	lexerOnce   sync.Once
	lexerFound  lexerFields
	lexerLayErr error
)

// lexerLayout checks once that ExternalLexer still has the fields lexerOrigin
// reads. Reading them with reflection keeps the recorder from calling
// Lookahead or Column, which would change the parser's read-frontier state.
func lexerLayout() (lexerFields, error) {
	lexerOnce.Do(func() {
		t := reflect.TypeOf((*gts.ExternalLexer)(nil)).Elem()
		field := func(parent reflect.Type, name string, kind reflect.Kind) (int, reflect.Type) {
			f, ok := parent.FieldByName(name)
			if !ok || f.Type.Kind() != kind {
				if lexerLayErr == nil {
					lexerLayErr = fmt.Errorf("scannercert: %s.%s is not a %s field; update lexer_link.go", parent, name, kind)
				}
				return 0, nil
			}
			return f.Index[0], f.Type
		}
		var point reflect.Type
		lexerFound.source, _ = field(t, "source", reflect.Slice)
		lexerFound.pos, _ = field(t, "pos", reflect.Int)
		lexerFound.point, point = field(t, "point", reflect.Struct)
		if point != nil {
			lexerFound.row, _ = field(point, "Row", reflect.Uint32)
			lexerFound.column, _ = field(point, "Column", reflect.Uint32)
		}
		if lexerLayErr == nil {
			if elem := t.Field(lexerFound.source).Type.Elem(); elem.Kind() != reflect.Uint8 {
				lexerLayErr = fmt.Errorf("scannercert: %s.source is not a byte slice; update lexer_link.go", t)
			}
		}
	})
	return lexerFound, lexerLayErr
}

// lexerOrigin returns the source and the cursor of a lexer that has not yet
// scanned. It panics if the layout check fails; Certify runs that check first
// and returns its error.
func lexerOrigin(l *gts.ExternalLexer) (source []byte, pos int, row, column uint32) {
	fields, err := lexerLayout()
	if err != nil {
		panic(err)
	}
	v := reflect.ValueOf(l).Elem()
	point := v.Field(fields.point)
	return v.Field(fields.source).Bytes(),
		int(v.Field(fields.pos).Int()),
		uint32(point.Field(fields.row).Uint()),
		uint32(point.Field(fields.column).Uint())
}
