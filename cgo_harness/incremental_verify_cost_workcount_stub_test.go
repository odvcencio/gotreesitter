//go:build cgo && treesitter_c_parity && !gts_workcount

package cgoharness

import "testing"

func verifyCostBeginWorkCount()         {}
func verifyCostEndWorkCount(*testing.T) {}
