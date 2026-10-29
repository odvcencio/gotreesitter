package main

import (
	"strings"
	"testing"
)

func TestValidateQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		lang  string
		want  string // substring of the error; empty means no error
	}{
		{"valid", "(function_declaration name: (identifier) @name body: (block) @body)", "go", ""},
		{"unknown node", "(no_such_node) @x", "go", "unknown node type"},
		{"unknown field", "(function_declaration nofield: (identifier) @n)", "go", "unknown field name"},
		{"unknown language", "(identifier) @x", "no_such_language", "unknown language"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQuery(tt.query, tt.lang)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
