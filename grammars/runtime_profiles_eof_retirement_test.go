package grammars

import (
	"sort"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestCompactGraduationGrantDenominatorAfterEOFRetirement(t *testing.T) {
	PurgeEmbeddedLanguageCache()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, test := range []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "http", load: HttpLanguage},
		{name: "robot", load: RobotLanguage},
	} {
		language := test.load()
		if language.CompactEOFAcceptNoActionSiblingsCertified {
			t.Errorf("loaded %s profile still sets the legacy EOF sibling bypass", test.name)
		}
	}

	activeGrammars := make(map[string]struct{})
	activeFlags := 0
	var eofSiblingGrants []string
	for name, profile := range builtinLanguageRuntimeProfiles {
		flags := [...]bool{
			profile.compactConvergedSplitDrops,
			profile.compactEOFAcceptNoActionSiblings,
			profile.compactPrimaryAcceptDerivation,
			profile.compactStrategy2ErrorRegion,
			profile.compactMissingTokenInsertion,
			profile.compactRecoveryPlainFirst,
		}
		for _, active := range flags {
			if !active {
				continue
			}
			activeFlags++
			activeGrammars[name] = struct{}{}
		}
		if profile.compactEOFAcceptNoActionSiblings {
			eofSiblingGrants = append(eofSiblingGrants, name)
		}
	}
	sort.Strings(eofSiblingGrants)

	if len(eofSiblingGrants) != 0 {
		t.Errorf("EOF sibling grants remain: %v", eofSiblingGrants)
	}
	if activeFlags != 22 || len(activeGrammars) != 13 {
		t.Errorf(
			"compact profile denominator is %d flags across %d grammars, want 22 across 13",
			activeFlags,
			len(activeGrammars),
		)
	}
}
