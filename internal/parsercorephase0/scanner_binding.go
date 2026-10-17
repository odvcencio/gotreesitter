package parsercorephase0

import "reflect"

// ScannerBindingUnchanged compares a captured interface descriptor after a
// capability callback without invoking scanner methods. Equal scalar values
// may share descriptors, while equal value boxes may conservatively differ.
// This is an additional rejection check; it never replaces scanner contract
// or checkpoint identity validation. Mutable payload through an unchanged
// scanner pointer remains permitted.
func ScannerBindingUnchanged(before, after any) bool {
	return reflect.ValueOf(before) == reflect.ValueOf(after)
}
