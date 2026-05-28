package gotreesitter

import "testing"

func TestTypeScriptMemberPrefixStartsWithModifier(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "public", src: "public value(): void", want: true},
		{name: "static spaced", src: " \n\tstatic value = 1", want: true},
		{name: "getter", src: "get value(): string", want: true},
		{name: "plain method", src: "value(): string", want: false},
		{name: "decorator", src: "@tracked public value(): void", want: false},
		{name: "numeric", src: "1: string", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typeScriptMemberPrefixStartsWithModifier([]byte(tt.src), 0, uint32(len(tt.src))); got != tt.want {
				t.Fatalf("typeScriptMemberPrefixStartsWithModifier(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestScanTypeScriptMemberPrefixTokensUsesRunningPoint(t *testing.T) {
	source := []byte("xx\n  public value(): void")
	tokens, ok := scanTypeScriptMemberPrefixTokens(source, 3, uint32(len(source)), Point{Row: 1})
	if !ok {
		t.Fatal("scanTypeScriptMemberPrefixTokens ok = false")
	}
	if got, want := len(tokens), 2; got != want {
		t.Fatalf("len(tokens) = %d, want %d", got, want)
	}
	if got, want := tokens[0].text, "public"; got != want {
		t.Fatalf("tokens[0].text = %q, want %q", got, want)
	}
	if got, want := tokens[0].startPoint, (Point{Row: 1, Column: 2}); got != want {
		t.Fatalf("tokens[0].startPoint = %+v, want %+v", got, want)
	}
	if got, want := tokens[0].endPoint, (Point{Row: 1, Column: 8}); got != want {
		t.Fatalf("tokens[0].endPoint = %+v, want %+v", got, want)
	}
	if got, want := tokens[1].text, "value"; got != want {
		t.Fatalf("tokens[1].text = %q, want %q", got, want)
	}
	if got, want := tokens[1].startPoint, (Point{Row: 1, Column: 9}); got != want {
		t.Fatalf("tokens[1].startPoint = %+v, want %+v", got, want)
	}
	if got, want := tokens[1].endPoint, (Point{Row: 1, Column: 14}); got != want {
		t.Fatalf("tokens[1].endPoint = %+v, want %+v", got, want)
	}
}
