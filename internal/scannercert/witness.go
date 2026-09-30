package scannercert

import (
	"bytes"
	"fmt"
)

// A sequence witness retains live state between scans. Restoring only the
// offending checkpoint would lose precisely the state an incomplete codec
// omitted, and could turn a real failure into an unreproducible report.
func (c *scannerCertification) shrinkSequence(kind string, traced *scannerCertificationPayload) (string, bool) {
	if len(traced.calls) == 0 {
		return "", false
	}
	calls := append([]scannerCertificationCall(nil), traced.calls...)
	source := bytes.Clone(calls[0].source)
	for i := range calls {
		if !bytes.Equal(calls[i].source, source) {
			return "", false
		}
		calls[i].valid = append([]bool(nil), calls[i].valid...)
	}
	useDirty := false
	dirty := c.lastReplay
	reproduces := func(source []byte, calls []scannerCertificationCall) (bool, string) {
		actual := c.ExternalScanner.Create()
		defer c.ExternalScanner.Destroy(actual)
		if traced.hasRestore {
			c.ExternalScanner.Deserialize(actual, bytes.Clone(traced.checkpoint))
		}
		actualCount := len(calls)
		if useDirty {
			actualCount-- // The last call seeds the independent dirty receiver.
		}
		for i, call := range calls[:actualCount] {
			lexer := c.api.New(source, call.offset)
			if i+1 < actualCount {
				_, fault := scannerCertificationScan(c.ExternalScanner, actual, lexer, call.valid)
				if fault != "" {
					return kind == "scan-panic", fault
				}
				continue
			}
			before := c.serialize(actual)
			restored := c.ExternalScanner.Create()
			defer c.ExternalScanner.Destroy(restored)
			if useDirty {
				seed := calls[len(calls)-1]
				c.ExternalScanner.Deserialize(restored, bytes.Clone(dirty.checkpoint))
				_, fault := scannerCertificationScan(c.ExternalScanner, restored, c.api.New(source, seed.offset), seed.valid)
				if fault != "" {
					return false, ""
				}
			}
			c.ExternalScanner.Deserialize(restored, bytes.Clone(before))
			if kind == "roundtrip" {
				after := c.serialize(restored)
				return !bytes.Equal(before, after), fmt.Sprintf("state=%x restored=%x", before, after)
			}
			clone := c.api.Clone(lexer)
			accepted, fault := scannerCertificationScan(c.ExternalScanner, actual, lexer, call.valid)
			replayed, replayFault := scannerCertificationScan(c.ExternalScanner, restored, clone, call.valid)
			if kind == "scan-panic" {
				return fault != "" || replayFault != "", fmt.Sprintf("original=%q restored=%q", fault, replayFault)
			}
			after, replayAfter := c.serialize(actual), c.serialize(restored)
			different := fault == "" && replayFault == "" && (accepted != replayed || !bytes.Equal(after, replayAfter) ||
				c.api.Observe(lexer) != c.api.Observe(clone))
			return different, fmt.Sprintf("state=%x accepted=%t/%t after=%x/%x", before, accepted, replayed, after, replayAfter)
		}
		return false, ""
	}
	if ok, _ := reproduces(source, calls); !ok {
		if dirty == nil || !bytes.Equal(dirty.call.source, source) {
			return "", false
		}
		useDirty = true
		seed := dirty.call
		seed.valid = append([]bool(nil), seed.valid...)
		calls = append(calls, seed)
		if ok, _ := reproduces(source, calls); !ok {
			return "", false
		}
	}
	// First remove unnecessary earlier scans, then minimize the shared source
	// and symbol masks. No grammar names or scanner payload fields enter this.
	minimum := 1
	if useDirty {
		minimum++
	}
	for i := 0; i+minimum < len(calls); {
		next := append(append([]scannerCertificationCall(nil), calls[:i]...), calls[i+1:]...)
		if ok, _ := reproduces(source, next); ok {
			calls = next
		} else {
			i++
		}
	}
	for chunk := max(1, len(source)/2); chunk > 0; chunk /= 2 {
		for start := 0; start+chunk <= len(source); {
			end := start + chunk
			nextCalls := append([]scannerCertificationCall(nil), calls...)
			crossesOrigin := false
			for i, call := range calls {
				if start < call.offset && end > call.offset {
					crossesOrigin = true
					break
				}
				if end <= call.offset {
					nextCalls[i].offset -= chunk
				}
			}
			if crossesOrigin {
				start++
				continue
			}
			nextSource := append(append([]byte(nil), source[:start]...), source[end:]...)
			if ok, _ := reproduces(nextSource, nextCalls); ok {
				source, calls = nextSource, nextCalls
			} else {
				start++
			}
		}
	}
	for _, call := range calls {
		for i, enabled := range call.valid {
			if enabled {
				call.valid[i] = false
				if ok, _ := reproduces(source, calls); !ok {
					call.valid[i] = true
				}
			}
		}
	}
	var offsets []int
	var valid [][]int
	for _, call := range calls {
		offsets = append(offsets, call.offset)
		var symbols []int
		for i, enabled := range call.valid {
			if enabled {
				symbols = append(symbols, i)
			}
		}
		valid = append(valid, symbols)
	}
	_, detail := reproduces(source, calls)
	dirtyDescription := ""
	if useDirty {
		dirtyDescription = fmt.Sprintf(" dirty-seed-last=true dirty-checkpoint=%x", dirty.checkpoint)
	}
	return fmt.Sprintf("input=%q offsets=%v valid=%v initial-checkpoint=%x restored-initial=%t%s %s", source, offsets, valid, traced.checkpoint, traced.hasRestore, dirtyDescription, detail), true
}
