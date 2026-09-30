//go:build cgo && treesitter_c_parity && !gts_workcount

package cgoharness

import "testing"

func cReuseBeginWorkCount()         {}
func cReuseEndWorkCount(*testing.T) {}
