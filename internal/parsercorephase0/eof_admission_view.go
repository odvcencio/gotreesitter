package parsercorephase0

import (
	"errors"
	"fmt"
)

const (
	// EOFAdmissionMaxTopPayloads bounds one exact accepted stack path.
	EOFAdmissionMaxTopPayloads = 4096
	// EOFAdmissionMetadataGroupSize is the producer poll cadence.
	EOFAdmissionMetadataGroupSize = 64
)

var (
	// ErrEOFAdmissionGenerationChanged reports stale immutable Core handles.
	ErrEOFAdmissionGenerationChanged = errors.New("parser-core phase zero: EOF admission Core generation changed")
	// ErrEOFAdmissionInexactPath reports a path with more than one history.
	ErrEOFAdmissionInexactPath = errors.New("parser-core phase zero: EOF admission path is not exact and singular")
	// ErrEOFAdmissionTopPayloadCap reports a path wider than the fixed cursor.
	ErrEOFAdmissionTopPayloadCap = errors.New("parser-core phase zero: EOF admission top payload cap")
	// ErrEOFAdmissionMalformed reports an invalid immutable arena record.
	ErrEOFAdmissionMalformed = errors.New("parser-core phase zero: malformed EOF admission record")
)

// EOFAdmissionSubtreeView exposes one immutable subtree record to a callback.
// Its slices alias Core arenas and remain valid only during that callback.
type EOFAdmissionSubtreeView struct {
	Generation            uint64
	Identity              SubtreeID
	Symbol                Symbol
	ProductionID          uint16
	DynamicPrecedence     int16
	StartByte             uint32
	EndByte               uint32
	Children              []SubtreeID
	Fields                []FieldMapEntry
	Aliases               []Symbol
	Extra                 bool
	External              bool
	Terminal              bool
	Fragile               bool
	Missing               bool
	MetadataAuthenticated bool
}

// EOFAdmissionExactPath reports the authenticated facts for one stack path.
type EOFAdmissionExactPath struct {
	Generation     uint64
	Payloads       uint32
	Links          uint32
	Polls          uint32
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
}

func (c *Core) checkEOFAdmissionGeneration(generation uint64) error {
	if c == nil || generation == 0 || c.AuthenticationGeneration() != generation {
		return ErrEOFAdmissionGenerationChanged
	}
	return nil
}

func (c *Core) pollEOFAdmissionGeneration(generation uint64, poll func() error) error {
	if err := c.checkEOFAdmissionGeneration(generation); err != nil {
		return err
	}
	if poll != nil {
		if err := poll(); err != nil {
			return err
		}
	}
	return c.checkEOFAdmissionGeneration(generation)
}

func eofAdmissionArenaRange(first, count uint32, total int, name string) (int, int, error) {
	end := uint64(first) + uint64(count)
	if end > uint64(total) {
		return 0, 0, fmt.Errorf("%w: %s range is outside its arena", ErrEOFAdmissionMalformed, name)
	}
	return int(first), int(end), nil
}

// VisitEOFAdmissionSubtree supplies one callback-scoped immutable record.
// It validates the Core generation before and after the callback.
func (c *Core) VisitEOFAdmissionSubtree(
	id SubtreeID,
	generation uint64,
	visit func(EOFAdmissionSubtreeView) error,
) error {
	if visit == nil {
		return errors.New("parser-core phase zero: EOF admission subtree visitor is nil")
	}
	if err := c.checkEOFAdmissionGeneration(generation); err != nil {
		return err
	}
	record, err := c.subtree(id)
	if err != nil {
		return err
	}
	childStart, childEnd, err := eofAdmissionArenaRange(
		record.firstChild,
		record.childCount,
		len(c.children),
		"child",
	)
	if err != nil {
		return err
	}
	fieldStart, fieldEnd, err := eofAdmissionArenaRange(
		record.firstField,
		record.fieldCount,
		len(c.fields),
		"field",
	)
	if err != nil {
		return err
	}
	aliasStart, aliasEnd, err := eofAdmissionArenaRange(
		record.firstAlias,
		record.aliasCount,
		len(c.aliases),
		"alias",
	)
	if err != nil {
		return err
	}
	if record.startByte > record.endByte {
		return fmt.Errorf("%w: subtree span is reversed", ErrEOFAdmissionMalformed)
	}
	if record.externalProvenanceState == subtreeExternalProvenanceReusedOpaque {
		return fmt.Errorf("%w: recovery admission cannot inspect a reused subtree", ErrEOFAdmissionMalformed)
	}
	view := EOFAdmissionSubtreeView{
		Generation:        generation,
		Identity:          id,
		Symbol:            record.symbol,
		ProductionID:      record.productionID,
		DynamicPrecedence: record.dynamicPrecedence,
		StartByte:         record.startByte,
		EndByte:           record.endByte,
		Children:          c.children[childStart:childEnd],
		Fields:            c.fields[fieldStart:fieldEnd],
		Aliases:           c.aliases[aliasStart:aliasEnd],
		Extra:             record.extra,
		External:          record.external,
		Terminal:          record.terminal,
		Fragile:           record.fragile,
		// S5 recovery stores the missing bit on its synthetic payload. The EOF
		// admission gate reads this view and declines that payload shape.
		Missing:               record.missing,
		MetadataAuthenticated: c.metadataConstructionAuthenticated,
	}
	if err := visit(view); err != nil {
		return err
	}
	return c.checkEOFAdmissionGeneration(generation)
}

// VisitEOFAdmissionExactPath visits one exact stack path in source order.
// The fixed reverse-link buffer makes the successful path allocation-free.
func (c *Core) VisitEOFAdmissionExactPath(
	head Head,
	generation uint64,
	poll func() error,
	visit func(ordinal uint32, payload SubtreeID) error,
) (EOFAdmissionExactPath, error) {
	var result EOFAdmissionExactPath
	result.Generation = generation
	if visit == nil {
		return result, errors.New("parser-core phase zero: EOF admission path visitor is nil")
	}
	if err := c.pollEOFAdmissionGeneration(generation, poll); err != nil {
		return result, err
	}
	result.Polls++
	var reverse [EOFAdmissionMaxTopPayloads]LinkID
	id := head.Node
	for {
		node, err := c.node(id)
		if err != nil {
			return result, err
		}
		if node.pathCount != 1 {
			return result, ErrEOFAdmissionInexactPath
		}
		if node.linkCount == 0 {
			break
		}
		if node.linkCount != 1 {
			return result, ErrEOFAdmissionInexactPath
		}
		if result.Links%EOFAdmissionMetadataGroupSize == 0 {
			if err := c.pollEOFAdmissionGeneration(generation, poll); err != nil {
				return result, err
			}
			result.Polls++
		}
		if result.Links >= EOFAdmissionMaxTopPayloads {
			return result, ErrEOFAdmissionTopPayloadCap
		}
		linkID := LinkID(node.firstLink)
		if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
			return result, fmt.Errorf("%w: link is outside its arena", ErrEOFAdmissionMalformed)
		}
		link := c.links[linkID-1]
		if link.next != 0 {
			return result, fmt.Errorf("%w: adjacency is not singular", ErrEOFAdmissionMalformed)
		}
		if link.prev == 0 || link.prev >= id {
			return result, fmt.Errorf("%w: predecessor does not decrease", ErrEOFAdmissionMalformed)
		}
		if err := link.validateShape(); err != nil {
			return result, fmt.Errorf("%w: %v", ErrEOFAdmissionMalformed, err)
		}
		if link.isRecoveryDiscontinuity() {
			return result, fmt.Errorf("%w: recovery discontinuity requires a recovery-aware visitor", ErrEOFAdmissionMalformed)
		}
		if link.payload != 0 && uint64(link.payload) > uint64(len(c.subtrees)) {
			return result, fmt.Errorf("%w: payload is outside its arena", ErrEOFAdmissionMalformed)
		}
		reverse[result.Links] = linkID
		result.Links++
		id = link.prev
	}
	for reverseIndex := int(result.Links) - 1; reverseIndex >= 0; reverseIndex-- {
		link := c.links[reverse[reverseIndex]-1]
		score, err := checkedAddScore(result.Score, link.scoreDelta)
		if err != nil {
			return result, err
		}
		result.Score = score
		if link.hasOrder() {
			result.BranchOrder = link.order
			result.HasBranchOrder = true
		}
		if link.payload != 0 {
			ordinal := result.Payloads
			result.Payloads++
			if err := visit(ordinal, link.payload); err != nil {
				return result, err
			}
		}
		if err := c.checkEOFAdmissionGeneration(generation); err != nil {
			return result, err
		}
	}
	if err := c.pollEOFAdmissionGeneration(generation, poll); err != nil {
		return result, err
	}
	result.Polls++
	return result, nil
}
