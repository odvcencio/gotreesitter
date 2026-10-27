//go:build !gts_parsercorephase0 && !gts_workcount && !gts_diag

package gotreesitter

// Qualification observations are completely absent from ordinary builds.
func observeTrustedGoReuseShadow(*Parser, []byte, *Tree, *Tree, *Tree, bool) {}

func observeAuthenticatedGoReuse(*Parser, []byte, *Tree, *Tree) {}
