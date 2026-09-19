package gotreesitter

import "testing"

// TestTreeExtraHandlesSaturate checks that the handle count stops at its
// maximum. Release must not free a saturated tree.
func TestTreeExtraHandlesSaturate(t *testing.T) {
	tree := &Tree{}
	for i := 0; i < int(maxTreeExtraHandles)+10; i++ {
		tree.retainUnchangedIncrementalResult()
	}
	if tree.extraHandles != maxTreeExtraHandles {
		t.Fatalf("extraHandles = %d, want %d", tree.extraHandles, maxTreeExtraHandles)
	}
	for i := 0; i < 20; i++ {
		tree.Release()
	}
	if tree.released {
		t.Fatal("Release freed a saturated tree")
	}
	if tree.extraHandles != maxTreeExtraHandles {
		t.Fatalf("Release changed a saturated count to %d", tree.extraHandles)
	}
}

// TestTreeRetainAfterReleaseDoesNothing checks that a released tree does not
// gain a handle.
func TestTreeRetainAfterReleaseDoesNothing(t *testing.T) {
	tree := &Tree{}
	tree.Release()
	tree.retainUnchangedIncrementalResult()
	if tree.extraHandles != 0 {
		t.Fatalf("released tree gained %d handles", tree.extraHandles)
	}
}
