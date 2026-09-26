package benchfixtures

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	ts "github.com/odvcencio/gotreesitter"
)

// EditSessionSeed identifies the fixed editor session sequence.
const EditSessionSeed uint32 = 4242
const EditSessionSteps = 72

// EditStep holds the complete source and edit coordinates after one operation.
type EditStep struct {
	Source []byte
	Edit   ts.InputEdit
}

// EditingSession returns 72 repeatable insert, delete, and replace edits.
func EditingSession(initial []byte) []EditStep {
	current := append([]byte(nil), initial...)
	state := EditSessionSeed
	steps := make([]EditStep, 0, EditSessionSteps)
	for i := 0; i < EditSessionSteps; i++ {
		state = state*1664525 + 1013904223
		at := 0
		if len(current) > 0 {
			at = int(state % uint32(len(current)))
		}
		start := sessionPoint(current, at)
		oldEnd := start
		newEnd := start
		oldEndByte := at
		newEndByte := at
		var next []byte
		switch i % 3 {
		case 0: // insert
			next = make([]byte, 0, len(current)+1)
			next = append(next, current[:at]...)
			next = append(next, 'x')
			next = append(next, current[at:]...)
			newEndByte++
		case 1: // delete
			if at < len(current) {
				oldEndByte++
			}
			next = append(append([]byte(nil), current[:at]...), current[oldEndByte:]...)
		case 2: // replace
			if at < len(current) {
				oldEndByte++
				newEndByte++
			}
			next = append([]byte(nil), current...)
			if at < len(next) {
				if next[at] == 'x' {
					next[at] = 'y'
				} else {
					next[at] = 'x'
				}
			}
		}
		oldEnd = sessionPoint(current, oldEndByte)
		newEnd = sessionPoint(next, newEndByte)
		steps = append(steps, EditStep{Source: next, Edit: ts.InputEdit{
			StartByte: uint32(at), OldEndByte: uint32(oldEndByte), NewEndByte: uint32(newEndByte),
			StartPoint: start, OldEndPoint: oldEnd, NewEndPoint: newEnd,
		}})
		current = next
	}
	return steps
}

func sessionPoint(source []byte, at int) ts.Point {
	before := source[:at]
	return ts.Point{Row: uint32(bytes.Count(before, []byte{'\n'})), Column: uint32(at - bytes.LastIndexByte(before, '\n') - 1)}
}

// EditingSessionSHA256 pins every edit coordinate and resulting source.
func EditingSessionSHA256(initial []byte) string {
	h := sha256.New()
	for _, step := range EditingSession(initial) {
		var fields [9 * 4]byte
		values := [...]uint32{
			step.Edit.StartByte, step.Edit.OldEndByte, step.Edit.NewEndByte,
			step.Edit.StartPoint.Row, step.Edit.StartPoint.Column,
			step.Edit.OldEndPoint.Row, step.Edit.OldEndPoint.Column,
			step.Edit.NewEndPoint.Row, step.Edit.NewEndPoint.Column,
		}
		for i, value := range values {
			binary.LittleEndian.PutUint32(fields[i*4:], value)
		}
		h.Write(fields[:])
		sum := sha256.Sum256(step.Source)
		h.Write(sum[:])
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
