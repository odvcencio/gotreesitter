package parsercorephase0

import (
	"errors"
	"fmt"
)

// NativeProjectionTiling checks raw byte coverage during the projection walk.
// A grammar root may contain gaps; every other nonterminal must cover its raw
// children and trailing extent with children or caller-approved trivia.
type NativeProjectionTiling struct {
	RootSymbol     Symbol
	HasRootSymbol  bool
	GapIsTolerated func([]byte) bool
}

// One frame owns projection, replay, and coverage progress for a raw subtree.
// No frame pointer survives a push, which may grow the backing array.
type nativeProjectionFrame struct {
	id, next, logicalStart, lastEnd uint32
	cursor, pre, state              StateID
	alias                           Symbol
	field                           FieldID
	preKnown, stateKnown, root      bool
}

func (scratch *MaterializationPostorderScratch) prepareNative(subtrees int) error {
	if err := scratch.prepareOwnershipColors(subtrees); err != nil {
		return err
	}
	if cap(scratch.nativeFrames) < 64 {
		scratch.nativeFrames = make([]nativeProjectionFrame, 0, 64)
	} else {
		scratch.nativeFrames = scratch.nativeFrames[:0]
	}
	return nil
}

func (t *NativeProjectionTiling) validateExtent(source []byte, start, end uint32) error {
	if end < start || uint64(end) > uint64(len(source)) {
		return errors.New("stream raw extent outside source")
	}
	return nil
}

// NativeProjectionCoverageError preserves a failed byte-coverage proof across
// the publication boundary. Callers may retry with recovery lexing, but may
// never publish the incomplete clean derivation.
type NativeProjectionCoverageError struct{ Start, End uint32 }

func (e *NativeProjectionCoverageError) Error() string {
	return fmt.Sprintf("stream accepted-leaf-tiling-gap %d..%d", e.Start, e.End)
}

func (t *NativeProjectionTiling) validateGap(source []byte, start, end uint32) error {
	if start < end && (t.GapIsTolerated == nil || !t.GapIsTolerated(source[start:end])) {
		return &NativeProjectionCoverageError{Start: start, End: end}
	}
	return nil
}
