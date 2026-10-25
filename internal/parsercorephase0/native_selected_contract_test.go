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
		{"9ad1f8d7d0f2b9aa32c61bd8295b56177caabb0ae2fb8dd4617559ee0f71fed4", true},
		{"214322327c9ee7dda29febf32e415f7ed9e00a90b7690d3c32522e393211b8da", true},
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
