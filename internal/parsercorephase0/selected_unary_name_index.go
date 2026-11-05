package parsercorephase0

import (
	"cmp"
	"slices"
)

// SelectedSymbolTextReader reads grammar symbol metadata, never language identity.
// Indexes passed to the helpers must be valid for the supplied reader.
type SelectedSymbolTextReader interface {
	SelectedSymbolText(Symbol) string
}

// SortSelectedSymbolNames orders symbol IDs by their exact bytewise symbol text.
func SortSelectedSymbolNames[R SelectedSymbolTextReader](ids []Symbol, text R) {
	slices.SortFunc(ids, func(a, b Symbol) int { return cmp.Compare(text.SelectedSymbolText(a), text.SelectedSymbolText(b)) })
}

// FindSelectedSymbolName returns the first position of name in a sorted index.
func FindSelectedSymbolName[R SelectedSymbolTextReader](ids []Symbol, name string, text R) (int, bool) {
	return slices.BinarySearchFunc(ids, name, func(id Symbol, want string) int { return cmp.Compare(text.SelectedSymbolText(id), want) })
}
