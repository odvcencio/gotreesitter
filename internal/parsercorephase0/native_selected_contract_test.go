package parsercorephase0

import (
	"encoding/hex"
	"testing"
)

func TestNativeSelectedSnapshotContracts(t *testing.T) {
	for _, tc := range []struct {
		digest    string
		byDefault bool
	}{
		{"df63fc35604c4e4e7a484abde9eb2110b61640045601c23991723f323a48310d", true},
		{"9ac37a326be7a4f4297efa6e43ba00317c5959e7b4909c9262043d408f284b30", false},
	} {
		bytes, err := hex.DecodeString(tc.digest)
		if err != nil {
			t.Fatal(err)
		}
		identity := [32]byte(bytes)
		if !NativeSelectedSnapshotContract(identity) || NativeSelectedSnapshotDefaultContract(identity) != tc.byDefault {
			t.Fatalf("unexpected publisher contract for %x", identity)
		}
		for i := range identity {
			changed := identity
			changed[i] ^= 1
			if NativeSelectedSnapshotContract(changed) || NativeSelectedSnapshotDefaultContract(changed) {
				t.Fatalf("unqualified table change admitted: %x", changed)
			}
		}
	}
}
