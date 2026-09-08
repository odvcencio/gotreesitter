package main

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestLanguageVersionSurvivesBlobGeneration(t *testing.T) {
	for _, version := range []int{14, 15} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			source := strings.Replace(miniParserC, "#define LANGUAGE_VERSION 14", fmt.Sprintf("#define LANGUAGE_VERSION %d", version), 1)
			grammar, err := ExtractGrammar(source)
			if err != nil {
				t.Fatal(err)
			}
			blob, err := EncodeLanguageBlob(BuildLanguage(grammar))
			if err != nil {
				t.Fatal(err)
			}
			language, err := gts.LoadLanguage(blob)
			if err != nil {
				t.Fatal(err)
			}
			if language.LanguageVersion != uint32(version) {
				t.Fatalf("language version=%d want=%d", language.LanguageVersion, version)
			}
		})
	}
}
