package parsercorephase0

import "testing"

func TestScannerBindingUnchangedKeepsIdentityWithoutReadingPayload(t *testing.T) {
	type scanner struct{ payload []byte }
	live := &scanner{payload: []byte{1}}
	var captured any = live
	live.payload[0] = 2
	if !ScannerBindingUnchanged(captured, live) {
		t.Fatal("mutable checkpoint payload changed its pointer binding")
	}
	if ScannerBindingUnchanged(captured, &scanner{payload: []byte{2}}) {
		t.Fatal("equal payload in another scanner retained the old binding")
	}
	if ScannerBindingUnchanged(captured, nil) || ScannerBindingUnchanged(nil, captured) || !ScannerBindingUnchanged(nil, nil) {
		t.Fatal("scanner presence was not preserved")
	}
}

func TestScannerBindingUnchangedHandlesNonComparableImplementations(t *testing.T) {
	for _, binding := range []any{[]byte{1}, map[string]int{"a": 1}, struct{ S []byte }{[]byte{2}}} {
		captured := binding
		if !ScannerBindingUnchanged(captured, binding) {
			t.Fatal("unchanged interface binding rejected")
		}
	}
	var a any = map[string]int{"a": 1}
	var b any = map[string]int{"a": 1}
	if ScannerBindingUnchanged(a, b) {
		t.Fatal("distinct non-comparable bindings matched")
	}
}

func TestScannerBindingUnchangedPreservesScalarTypeAndValue(t *testing.T) {
	type scanner uint8
	var original any = scanner(17)
	captured := original
	if !ScannerBindingUnchanged(captured, original) {
		t.Fatal("unchanged scalar binding rejected")
	}
	if ScannerBindingUnchanged(captured, scanner(18)) || ScannerBindingUnchanged(captured, uint8(17)) {
		t.Fatal("changed scalar identity retained the old descriptor")
	}
	// A reboxed equal scalar may share its descriptor. Its unchanged type and
	// value retain the existing scanner fingerprint identity in either case.
}
