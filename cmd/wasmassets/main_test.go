package main

import "testing"

func TestRuntimeBuildTags(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     string
		wantErr  bool
	}{
		{name: "go", language: "go", want: "grammar_subset,grammar_subset_go"},
		{name: "underscore", language: "c_sharp", want: "grammar_subset,grammar_subset_c_sharp"},
		{name: "empty", wantErr: true},
		{name: "comma", language: "go,grammar_subset_python", wantErr: true},
		{name: "path", language: "../go", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := runtimeBuildTags(test.language)
			if (err != nil) != test.wantErr {
				t.Fatalf("runtimeBuildTags(%q) error = %v, wantErr %v", test.language, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("runtimeBuildTags(%q) = %q, want %q", test.language, got, test.want)
			}
		})
	}
}
