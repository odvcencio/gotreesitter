package corpuscheck

import (
	"strings"
	"testing"
)

func TestParseFile_Basic(t *testing.T) {
	data := []byte(`==================
Arrays
==================

[1, 2]

--------------------

(document
  (array
    (number)
    (number)))
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	tc := cases[0]
	if tc.Name != "Arrays" {
		t.Errorf("Name = %q, want Arrays", tc.Name)
	}
	if string(tc.Input) != "[1, 2]" {
		t.Errorf("Input = %q", tc.Input)
	}
	wantExpected := "(document\n  (array\n    (number)\n    (number)))"
	if tc.Expected != wantExpected {
		t.Errorf("Expected = %q, want %q", tc.Expected, wantExpected)
	}
}

func TestParseFile_MultipleTests(t *testing.T) {
	data := []byte(`===
first
===

a

---

(a)

===
second
===

b

---

(b)
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("want 2 cases, got %d", len(cases))
	}
	if cases[0].Name != "first" || cases[1].Name != "second" {
		t.Fatalf("names = %q, %q", cases[0].Name, cases[1].Name)
	}
	if string(cases[0].Input) != "a" || string(cases[1].Input) != "b" {
		t.Fatalf("inputs = %q, %q", cases[0].Input, cases[1].Input)
	}
}

func TestParseFile_Attributes(t *testing.T) {
	data := []byte(`====================
Type assertions
:language(typescript)
====================

<A>b;

--

(program)
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	tc := cases[0]
	if !tc.HasAttr("language") {
		t.Fatalf("expected :language attribute, got %v", tc.Attrs)
	}
	val, ok := tc.AttrValue("language")
	if !ok || val != "typescript" {
		t.Fatalf("AttrValue(language) = %q, %v", val, ok)
	}
}

func TestParseFile_SkipAndErrorAttributes(t *testing.T) {
	data := []byte(`===
broken
:error
:skip
===

(2

---
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	tc := cases[0]
	if !tc.HasAttr("error") || !tc.HasAttr("skip") {
		t.Fatalf("attrs = %v, want error and skip", tc.Attrs)
	}
	if strings.TrimSpace(tc.Expected) != "" {
		t.Fatalf("Expected = %q, want empty", tc.Expected)
	}
}

// TestParseFile_SuffixDisambiguation covers the convention several
// grammars (yaml, tlaplus, liquid) use so a body containing a literal
// "---" or "====" line isn't mistaken for the divider.
func TestParseFile_SuffixDisambiguation(t *testing.T) {
	data := []byte(`==================||||
Front Matter Only
==================||||
---
prop: value
---
---||||

(program
  (front_matter))
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	tc := cases[0]
	wantInput := "---\nprop: value\n---"
	if string(tc.Input) != wantInput {
		t.Fatalf("Input = %q, want %q", tc.Input, wantInput)
	}
	wantExpected := "(program\n  (front_matter))"
	if tc.Expected != wantExpected {
		t.Fatalf("Expected = %q, want %q", tc.Expected, wantExpected)
	}
}

// TestParseFile_ColonBodyContentIsNotAnAttribute covers Ruby/Clojure/Lisp
// symbol literals (":foo") that must not be mistaken for header
// attributes when they appear in the source body.
func TestParseFile_ColonBodyContentIsNotAnAttribute(t *testing.T) {
	data := []byte(`===
symbol
===

:foo

---

(program
  (symbol))
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	tc := cases[0]
	if len(tc.Attrs) != 0 {
		t.Fatalf("Attrs = %v, want none (body :foo is not an attribute)", tc.Attrs)
	}
	if string(tc.Input) != ":foo" {
		t.Fatalf("Input = %q, want %q", tc.Input, ":foo")
	}
}

func TestParseFile_CRLF(t *testing.T) {
	data := []byte("===\r\nCRLF test\r\n===\r\n\r\nputs 'hi'\r\n\r\n---\r\n\r\n(program)\r\n")
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	if string(cases[0].Input) != "puts 'hi'" {
		t.Fatalf("Input = %q", cases[0].Input)
	}
	if cases[0].Expected != "(program)" {
		t.Fatalf("Expected = %q", cases[0].Expected)
	}
}

func TestParseFile_VaryingMarkerLength(t *testing.T) {
	data := []byte(`=========================
long open, short close
===

x

-----------
(a)
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	if string(cases[0].Input) != "x" {
		t.Fatalf("Input = %q", cases[0].Input)
	}
}

func TestParseFile_AnonymousName(t *testing.T) {
	// Some fixtures have a blank line immediately between the two header
	// markers (no name at all).
	data := []byte(`===

===

x

---

(a)
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("want 1 case, got %d", len(cases))
	}
	if cases[0].Name != "" {
		t.Fatalf("Name = %q, want empty", cases[0].Name)
	}
}

func TestParseFile_MalformedTestRecovers(t *testing.T) {
	// The first "test" here never opens with a marker; ParseFile should
	// recover and still parse the second, well-formed test.
	data := []byte(`some stray line that is not a marker

===
second
===

b

---

(b)
`)
	cases, err := ParseFile(data)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var withErr, ok int
	for _, tc := range cases {
		if tc.ParseError != nil {
			withErr++
		} else {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("want 1 successfully parsed case, got %d (cases=%+v)", ok, cases)
	}
}

func TestTestCase_HasAttr(t *testing.T) {
	tc := TestCase{Attrs: []string{"skip", "language(tsx)"}}
	if !tc.HasAttr("skip") {
		t.Errorf("HasAttr(skip) = false")
	}
	if !tc.HasAttr("language") {
		t.Errorf("HasAttr(language) = false")
	}
	if tc.HasAttr("error") {
		t.Errorf("HasAttr(error) = true, want false")
	}
	if v, ok := tc.AttrValue("language"); !ok || v != "tsx" {
		t.Errorf("AttrValue(language) = %q, %v", v, ok)
	}
}
