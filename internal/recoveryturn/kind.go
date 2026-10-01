// Package recoveryturn distinguishes versions advanced before their native
// physical dispatch turn by the shared-lookahead recovery loop.
package recoveryturn

// Kind fits in the recovery stack header's existing one-byte marker.
type Kind uint8

const (
	None Kind = iota
	Missing
	Resync
)
