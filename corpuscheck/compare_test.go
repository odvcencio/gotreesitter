package corpuscheck

import "testing"

func TestCompare_Match(t *testing.T) {
	a, err := ParseExpected(`(document (array (number) (number)))`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseExpected(`(document (array (number) (number)))`)
	if err != nil {
		t.Fatal(err)
	}
	if d := Compare(a, b); d != nil {
		t.Fatalf("expected match, got %v", d)
	}
}

func TestCompare_TypeMismatch(t *testing.T) {
	a, _ := ParseExpected(`(document (array (number)))`)
	b, _ := ParseExpected(`(document (array (string)))`)
	d := Compare(a, b)
	if d == nil || d.Category != CategoryType {
		t.Fatalf("d = %v, want type mismatch", d)
	}
}

func TestCompare_ShapeMismatch(t *testing.T) {
	a, _ := ParseExpected(`(document (array (number) (number)))`)
	b, _ := ParseExpected(`(document (array (number)))`)
	d := Compare(a, b)
	if d == nil || d.Category != CategoryShape {
		t.Fatalf("d = %v, want shape mismatch", d)
	}
}

func TestCompare_FieldMismatch(t *testing.T) {
	a, _ := ParseExpected(`(assignment left: (identifier) right: (identifier))`)
	b, _ := ParseExpected(`(assignment left: (identifier) value: (identifier))`)
	d := Compare(a, b)
	if d == nil || d.Category != CategoryField {
		t.Fatalf("d = %v, want field mismatch", d)
	}
}

func TestCompare_MissingMismatch(t *testing.T) {
	a, _ := ParseExpected(`(a (b (MISSING ";")))`)
	b, _ := ParseExpected(`(a (b (c)))`)
	d := Compare(a, b)
	if d == nil || d.Category != CategoryMissing {
		t.Fatalf("d = %v, want missing mismatch", d)
	}
}

func TestCompare_MissingValueMismatch(t *testing.T) {
	a, _ := ParseExpected(`(a (MISSING ";"))`)
	b, _ := ParseExpected(`(a (MISSING ","))`)
	d := Compare(a, b)
	if d == nil || d.Category != CategoryMissingValue {
		t.Fatalf("d = %v, want missing_value mismatch", d)
	}
}

func TestCompare_UnexpectedUnsupported(t *testing.T) {
	expected, _ := ParseExpected(`(a (ERROR (UNEXPECTED '+')))`)
	actual, _ := ParseExpected(`(a (ERROR))`)
	d := Compare(expected, actual)
	if d == nil || d.Category != CategoryUnexpectedUnsupported {
		t.Fatalf("d = %v, want unexpected_node_unsupported", d)
	}
}
