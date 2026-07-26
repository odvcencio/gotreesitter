package corpuscheck

import "testing"

func TestParseExpected_Basic(t *testing.T) {
	n, err := ParseExpected(`(document
  (array
    (number)
    (number)))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	if n.Type != "document" {
		t.Fatalf("root type = %q", n.Type)
	}
	if len(n.Children) != 1 || n.Children[0].Type != "array" {
		t.Fatalf("children = %+v", n.Children)
	}
	arr := n.Children[0]
	if len(arr.Children) != 2 || arr.Children[0].Type != "number" || arr.Children[1].Type != "number" {
		t.Fatalf("array children = %+v", arr.Children)
	}
}

func TestParseExpected_Fields(t *testing.T) {
	n, err := ParseExpected(`(module
  (assignment
    left: (identifier)
    right: (binary_operator
      left: (identifier)
      right: (identifier))))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	assign := n.Children[0]
	if assign.Type != "assignment" {
		t.Fatalf("type = %q", assign.Type)
	}
	if len(assign.Children) != 2 {
		t.Fatalf("children = %+v", assign.Children)
	}
	if assign.Children[0].Field != "left" || assign.Children[1].Field != "right" {
		t.Fatalf("fields = %q, %q", assign.Children[0].Field, assign.Children[1].Field)
	}
	rhs := assign.Children[1]
	if rhs.Type != "binary_operator" || len(rhs.Children) != 2 {
		t.Fatalf("rhs = %+v", rhs)
	}
}

func TestParseExpected_ErrorNode(t *testing.T) {
	n, err := ParseExpected(`(source_file
  (function_declaration
    (identifier)
    (parameter_list)
    (block
      (ERROR
        (identifier))
      (comment))))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	block := n.Children[0].Children[2]
	if block.Type != "block" {
		t.Fatalf("type = %q", block.Type)
	}
	if block.Children[0].Type != "ERROR" {
		t.Fatalf("first child = %q, want ERROR", block.Children[0].Type)
	}
	if len(block.Children[0].Children) != 1 || block.Children[0].Children[0].Type != "identifier" {
		t.Fatalf("ERROR children = %+v", block.Children[0].Children)
	}
}

func TestParseExpected_EmptyErrorNode(t *testing.T) {
	n, err := ParseExpected(`(a (ERROR) (b))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	if len(n.Children) != 2 || n.Children[0].Type != "ERROR" || len(n.Children[0].Children) != 0 {
		t.Fatalf("children = %+v", n.Children)
	}
}

func TestParseExpected_MissingAnonymousToken(t *testing.T) {
	n, err := ParseExpected(`(a (b (MISSING ";")))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	m := n.Children[0].Children[0]
	if !m.Missing || !m.MissingIsAnon || m.Type != ";" {
		t.Fatalf("missing node = %+v", m)
	}
}

func TestParseExpected_MissingNamedNode(t *testing.T) {
	n, err := ParseExpected(`(a (MISSING identifier))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	m := n.Children[0]
	if !m.Missing || m.MissingIsAnon || m.Type != "identifier" {
		t.Fatalf("missing node = %+v", m)
	}
}

func TestParseExpected_MissingEmbeddedQuote(t *testing.T) {
	// (MISSING """) means the missing anonymous token's own text is a
	// single '"' character -- the fixture format doesn't escape it.
	n, err := ParseExpected(`(a (MISSING """))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	m := n.Children[0]
	if !m.Missing || !m.MissingIsAnon || m.Type != `"` {
		t.Fatalf("missing node = %+v", m)
	}
}

func TestParseExpected_Unexpected(t *testing.T) {
	n, err := ParseExpected(`(a (ERROR (UNEXPECTED '+')))`)
	if err != nil {
		t.Fatalf("ParseExpected: %v", err)
	}
	errNode := n.Children[0]
	if len(errNode.Children) != 1 || !errNode.Children[0].Unexpected {
		t.Fatalf("children = %+v", errNode.Children)
	}
}

func TestParseExpected_TrailingContentIsError(t *testing.T) {
	_, err := ParseExpected(`(a) (b)`)
	if err == nil {
		t.Fatalf("expected error for trailing content")
	}
}

func TestParseExpected_MalformedInput(t *testing.T) {
	if _, err := ParseExpected(``); err == nil {
		t.Fatalf("expected error for empty input")
	}
	if _, err := ParseExpected(`(a`); err == nil {
		t.Fatalf("expected error for unterminated node")
	}
	if _, err := ParseExpected(`a)`); err == nil {
		t.Fatalf("expected error for missing opening paren")
	}
}
