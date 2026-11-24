//go:build !grammar_subset

package grammarruntime

import (
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// TestYamlScannerIsRegisteredAndBindsThroughTheCore: the port was compared with the pinned
// C scanner call by call (cgo_harness scandiff, mode on); this test pins the registration
// that follows from it. A Language holds the bound *YamlExternalScanner.
func TestYamlScannerIsRegisteredAndBindsThroughTheCore(t *testing.T) {
	checkRegisteredScanner(t, "yaml", &YamlExternalScanner{}, true, func(payload any) bool { return payload.(*yamlPayload).cFaithful })
}

// TestYamlScannerSnapshotOwnsItsBinding: the core's private snapshot carries the
// bound symbol map and token order, compares equal to its source, and shares no
// slice with it.
func TestYamlScannerSnapshotOwnsItsBinding(t *testing.T) {
	sc, ok := YamlLanguage().ExternalScanner.(*YamlExternalScanner)
	if !ok || sc.binding == nil {
		t.Fatalf("YAML language scanner %T is not a bound *YamlExternalScanner", YamlLanguage().ExternalScanner)
	}
	reg, ok := scanmode.Lookup(sc)
	if !ok || reg.SameConfig == nil {
		t.Fatalf("registration %+v (found %v), want a configuration comparison", reg, ok)
	}
	private, key, ok := reg.Snapshot(sc)
	if !ok || len(key) == 0 {
		t.Fatalf("snapshot ok=%v key=%d bytes", ok, len(key))
	}
	copied, ok := private.(*YamlExternalScanner)
	if !ok || copied == sc || copied.binding == sc.binding {
		t.Fatal("the snapshot shares the scanner or its binding")
	}
	if !reflect.DeepEqual(copied.binding, sc.binding) || !reg.SameConfig(sc, copied) {
		t.Fatal("the snapshot's binding differs from its source")
	}
	if len(sc.binding.externalToToken) > 0 && &copied.binding.externalToToken[0] == &sc.binding.externalToToken[0] {
		t.Fatal("the snapshot shares the token order slice")
	}
	changed := *copied.binding
	changed.symbols[0]++
	if reg.SameConfig(sc, &YamlExternalScanner{binding: &changed}) {
		t.Fatal("a different symbol map compared equal")
	}
}
