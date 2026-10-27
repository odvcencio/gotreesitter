//go:build gts_parsercorephase0 || gts_workcount || gts_diag

package gotreesitter

import "testing"

// RunLanguageCopyDoesNotAuthenticate checks the producer boundary, before
// any tree edit: copies and caller-constructed metadata are not the artifact.
func RunLanguageCopyDoesNotAuthenticate(t *testing.T, original *Language) {
	for _, variant := range []string{"copy", "modified-symbol-table", "forged-receipt"} {
		t.Run(variant, func(t *testing.T) {
			clone := *original
			switch variant {
			case "modified-symbol-table":
				clone.SymbolNames = append([]string(nil), original.SymbolNames...)
				clone.SymbolNames[0] += "_modified"
			case "forged-receipt":
				clone.SmallSourceReuseShadowArtifact = [32]byte{1}
			}
			parser := NewParser(&clone)
			tree, err := parser.Parse([]byte("package p\nvar value = 1\n"))
			if err != nil || tree == nil {
				t.Fatalf("parse: %v", err)
			}
			defer tree.Release()
			if key := tree.arena.legacyReuseReads.DefaultProducerSchedule(len(tree.source)); key != 0 {
				t.Fatalf("untrusted %s acquired producer key %d", variant, key)
			}
		})
	}
}

func AssertLanguageProducerKey(t *testing.T, language *Language, want bool) {
	t.Helper()
	tree, err := NewParser(language).Parse([]byte("package p\nvar value = 1\n"))
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	if key := tree.arena.legacyReuseReads.DefaultProducerSchedule(len(tree.source)); (key != 0) != want {
		t.Fatalf("producer key %d, expected present=%t", key, want)
	}
}
