package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
)

// LinkRange identifies one complete, bounded adjacency chain. First names the
// newest link and Count includes every link through the zero next pointer.
type LinkRange struct {
	First LinkID
	Count uint32
}

// Empty reports whether both range fields use their zero value.
func (r LinkRange) Empty() bool { return r.First == 0 && r.Count == 0 }

// LinkChainRef identifies one published graph adjacency chain. First names
// the newest link, and Count bounds traversal through link.next. Link IDs need
// not be physically contiguous.
type LinkChainRef struct {
	First LinkID
	Count uint32
}

// Empty reports whether both chain reference fields use their zero value.
func (r LinkChainRef) Empty() bool { return r.First == 0 && r.Count == 0 }

// appendGraphLink keeps the optional sidecar aligned with the link arena. A
// newly published link starts unbound; authentication binds it later.
func (c *Core) appendGraphLink(link linkRecord) LinkID {
	id := LinkID(len(c.links) + 1)
	c.links = append(c.links, link)
	if c.dropCohortLinkRefIndexes != nil {
		c.dropCohortLinkRefIndexes = append(c.dropCohortLinkRefIndexes, 0)
	}
	return id
}

// resolveDropCohortLinkRange authenticates one exact graph adjacency chain.
// The production graph prepends links, so valid next identifiers decrease by
// one and the final link points to zero.
func (c *Core) resolveDropCohortLinkRange(r LinkRange) ([]LinkID, error) {
	if c == nil {
		return nil, errors.New("parser-core phase zero: link range on nil core")
	}
	if r.Empty() {
		return nil, nil
	}
	if r.First == 0 || r.Count == 0 {
		return nil, errors.New("parser-core phase zero: mixed-empty link range")
	}
	if uint64(r.Count) > uint64(c.limits.MaxLinksPerBoundary) {
		return nil, errors.New("parser-core phase zero: link range exceeds boundary cap")
	}
	if uint64(r.First) > uint64(len(c.links)) {
		return nil, errors.New("parser-core phase zero: link range starts outside the graph")
	}
	ids := make([]LinkID, r.Count)
	current := r.First
	for index := range ids {
		if current == 0 || uint64(current) > uint64(len(c.links)) {
			return nil, errors.New("parser-core phase zero: link range leaves the graph")
		}
		link := c.links[current-1]
		if err := link.validateShape(); err != nil {
			return nil, err
		}
		if link.prev == 0 || uint64(link.prev) > uint64(len(c.nodes)) {
			return nil, errors.New("parser-core phase zero: link range has an invalid predecessor")
		}
		ids[index] = current
		if index+1 == len(ids) {
			if link.next != 0 {
				return nil, errors.New("parser-core phase zero: link range does not terminate at the graph boundary")
			}
			break
		}
		if current <= 1 || link.next != current-1 {
			return nil, errors.New("parser-core phase zero: link range is noncontiguous")
		}
		current = link.next
	}
	return ids, nil
}

// LinkRangeForHead returns the authenticated adjacency range stored on head.
// This helper is cold-path metadata access; it does not affect routing.
func (c *Core) LinkRangeForHead(head Head) (LinkRange, error) {
	if c == nil {
		return LinkRange{}, errors.New("parser-core phase zero: link range on nil core")
	}
	node, err := c.node(head.Node)
	if err != nil {
		return LinkRange{}, err
	}
	rangeValue := LinkRange{First: LinkID(node.firstLink), Count: node.linkCount}
	if rangeValue.Empty() {
		return LinkRange{}, nil
	}
	if _, err := c.resolveDropCohortLinkRange(rangeValue); err != nil {
		return LinkRange{}, err
	}
	return rangeValue, nil
}

// reductionLinkChainForHead returns the O(1) chain metadata for a reduction
// output. appendNodeAt authenticates the complete chain before publication,
// so this handoff reads the stored first link and count. It does not allocate,
// require physical LinkID contiguity, or walk adjacency.
func (c *Core) reductionLinkChainForHead(head Head) (LinkChainRef, error) {
	if c == nil {
		return LinkChainRef{}, errors.New("parser-core phase zero: reduction link chain on nil core")
	}
	node, err := c.node(head.Node)
	if err != nil {
		return LinkChainRef{}, err
	}
	chain := LinkChainRef{First: LinkID(node.firstLink), Count: node.linkCount}
	if node.firstLink == 0 && node.linkCount == 0 {
		return LinkChainRef{}, nil
	}
	if chain.First == 0 || chain.Count == 0 {
		return LinkChainRef{}, errors.New("parser-core phase zero: reduction link chain is mixed-empty")
	}
	if uint64(chain.First) > uint64(len(c.links)) {
		return LinkChainRef{}, errors.New("parser-core phase zero: reduction link chain starts outside the graph")
	}
	if uint64(chain.Count) > uint64(len(c.links)) {
		return LinkChainRef{}, errors.New("parser-core phase zero: reduction link chain exceeds the graph")
	}
	return chain, nil
}

// DropCohortLinkRefIndex returns the one-based certificate index attached to
// link. It fails closed for an unbound or malformed sidecar entry.
func (c *Core) DropCohortLinkRefIndex(link LinkID) (uint32, bool) {
	if c == nil || link == 0 || uint64(link) > uint64(len(c.links)) ||
		len(c.dropCohortLinkRefIndexes) > len(c.links) ||
		uint64(link) > uint64(len(c.dropCohortLinkRefIndexes)) {
		return 0, false
	}
	index := c.dropCohortLinkRefIndexes[link-1]
	if index == 0 || uint64(index) > uint64(len(c.dropCohortCertificateRefs)) {
		return 0, false
	}
	return index, true
}

// finalizedDropCohortCertificateIndex proves one current, complete reference
// and returns its one-based certificate-store index.
func (c *Core) finalizedDropCohortCertificateIndex(ref DropCohortRef) (uint32, DropCohortHandle, bool) {
	if c == nil {
		return 0, DropCohortHandle{}, false
	}
	recordIndex, handle, reason := c.dropCohortVerifierLookup(ref, false)
	if reason != dropCohortVerifierProved || recordIndex < 0 || recordIndex >= len(c.dropCohortRecords) {
		return 0, handle, false
	}
	record := c.dropCohortRecords[recordIndex]
	if record.handle != handle || dropCohortVerifierClassifyRecord(record) != dropCohortVerifierProved {
		return 0, handle, false
	}
	if _, ok := c.findDropCohortMember(record, ref.Branch); !ok {
		return 0, handle, false
	}
	for index, stored := range c.dropCohortCertificateRefs {
		if stored != ref {
			continue
		}
		if uint64(index) >= uint64(math.MaxUint32) {
			return 0, handle, false
		}
		return uint32(index + 1), handle, true
	}
	return 0, handle, false
}

// DropCohortLinkRef returns the finalized reference attached to link.
func (c *Core) DropCohortLinkRef(link LinkID) (DropCohortRef, bool) {
	index, ok := c.DropCohortLinkRefIndex(link)
	if !ok {
		return DropCohortRef{}, false
	}
	ref := c.dropCohortCertificateRefs[index-1]
	certificateIndex, _, ok := c.finalizedDropCohortCertificateIndex(ref)
	if !ok || certificateIndex != index {
		return DropCohortRef{}, false
	}
	return ref, true
}

func equalReductionPlan(left, right ReductionPlan) bool {
	if left.productionID != right.productionID || left.childCount != right.childCount ||
		len(left.fields) != len(right.fields) || len(left.aliases) != len(right.aliases) {
		return false
	}
	for index := range left.fields {
		if left.fields[index] != right.fields[index] {
			return false
		}
	}
	for index := range left.aliases {
		if left.aliases[index] != right.aliases[index] {
			return false
		}
	}
	return true
}

// authenticatedDropCohortAction derives action identity from authenticated
// scheduler inputs. Callers cannot supply an unchecked identity value.
func (c *Core) authenticatedDropCohortAction(boundary ClassifiedBoundary, descriptor ActionRowDescriptor, selectedOrdinal int, plan ReductionPlan) (DropCohortActionIdentity, error) {
	if err := c.validateClassification(boundary); err != nil {
		return DropCohortActionIdentity{}, err
	}
	if descriptor != boundary.actions.Descriptor() {
		return DropCohortActionIdentity{}, errors.New("parser-core phase zero: link bind action descriptor mismatch")
	}
	act, err := c.classifiedActionRef(boundary, selectedOrdinal)
	if err != nil {
		return DropCohortActionIdentity{}, err
	}
	if act.Type != ActionReduce || !descriptor.HasReduce() {
		return DropCohortActionIdentity{}, errors.New("parser-core phase zero: link bind action is not a reduction")
	}
	authenticatedPlan, err := c.reductionPlan(*act)
	if err != nil {
		return DropCohortActionIdentity{}, err
	}
	if !equalReductionPlan(plan, authenticatedPlan) {
		return DropCohortActionIdentity{}, errors.New("parser-core phase zero: link bind reduction plan mismatch")
	}
	if selectedOrdinal > math.MaxInt32 {
		return DropCohortActionIdentity{}, errors.New("parser-core phase zero: link bind action ordinal overflow")
	}
	return DropCohortActionIdentity{
		BoundaryState: boundary.state,
		Lookahead:     boundary.lookahead,
		ActionOrdinal: int32(selectedOrdinal),
		Action:        *act,
		NoLookahead:   c.reduceNoLookaheadContext,
		Selection:     c.dropCohortSelectionContext,
	}, nil
}

func (c *Core) bindDropCohortLinkRefsOwned(owner SchedulerTransactionToken, boundary ClassifiedBoundary, descriptor ActionRowDescriptor, selectedOrdinal int, plan ReductionPlan, links LinkRange, refs []DropCohortRef) error {
	if err := c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	return c.finishSchedulerOwned(owner, c.bindDropCohortLinkRefs(boundary, descriptor, selectedOrdinal, plan, links, refs))
}

func (c *Core) bindDropCohortLinkRefs(boundary ClassifiedBoundary, descriptor ActionRowDescriptor, selectedOrdinal int, plan ReductionPlan, links LinkRange, refs []DropCohortRef) error {
	if c == nil {
		return errors.New("parser-core phase zero: link bind on nil core")
	}
	expectedAction, err := c.authenticatedDropCohortAction(boundary, descriptor, selectedOrdinal, plan)
	if err != nil {
		return err
	}
	linkIDs, err := c.resolveDropCohortLinkRange(links)
	if err != nil {
		return err
	}
	if len(refs) != len(linkIDs) {
		return fmt.Errorf("parser-core phase zero: link bind reference count %d does not match link count %d", len(refs), len(linkIDs))
	}
	if len(linkIDs) == 0 {
		return nil
	}
	if c.dropCohortLinkRefIndexes != nil && len(c.dropCohortLinkRefIndexes) > len(c.links) {
		return errors.New("parser-core phase zero: link bind sidecar is longer than the graph")
	}
	indexes := make([]uint32, len(refs))
	for index, ref := range refs {
		certificateIndex, _, ok := c.finalizedDropCohortCertificateIndex(ref)
		if !ok {
			return errors.New("parser-core phase zero: link bind reference is not finalized")
		}
		recordIndex, handle, reason := c.dropCohortVerifierLookup(ref, false)
		if reason != dropCohortVerifierProved || recordIndex < 0 || recordIndex >= len(c.dropCohortRecords) {
			return errors.New("parser-core phase zero: link bind reference is not finalized")
		}
		record := c.dropCohortRecords[recordIndex]
		memberIndex, ok := c.findDropCohortMember(record, ref.Branch)
		if !ok || memberIndex < 0 || memberIndex >= len(c.dropCohortMembers) {
			return errors.New("parser-core phase zero: link bind member is missing")
		}
		member := c.dropCohortMembers[memberIndex]
		if member.cohort != handle || member.action != expectedAction {
			return errors.New("parser-core phase zero: link bind action authentication failed")
		}
		link := c.links[linkIDs[index]-1]
		if member.head.Node != link.prev {
			return errors.New("parser-core phase zero: link bind graph identity mismatch")
		}
		indexes[index] = certificateIndex
		if prior := c.sidecarValue(linkIDs[index]); prior != 0 && prior != certificateIndex {
			return errors.New("parser-core phase zero: link bind would overwrite a different certificate")
		}
	}
	if c.dropCohortLinkRefIndexes == nil {
		c.dropCohortLinkRefIndexes = make([]uint32, len(c.links))
	} else if len(c.dropCohortLinkRefIndexes) < len(c.links) {
		c.dropCohortLinkRefIndexes = append(c.dropCohortLinkRefIndexes, make([]uint32, len(c.links)-len(c.dropCohortLinkRefIndexes))...)
	}
	for index, certificateIndex := range indexes {
		linkIndex := int(linkIDs[index]) - 1
		prior := c.dropCohortLinkRefIndexes[linkIndex]
		if prior == certificateIndex {
			continue
		}
		if len(c.transactions) != 0 {
			c.dropCohortLinkRefJournal = append(c.dropCohortLinkRefJournal, dropCohortLinkRefMutation{index: linkIndex, previous: prior})
		}
		c.dropCohortLinkRefIndexes[linkIndex] = certificateIndex
	}
	return nil
}

func (c *Core) sidecarValue(link LinkID) uint32 {
	if c == nil || link == 0 || uint64(link) > uint64(len(c.dropCohortLinkRefIndexes)) {
		return 0
	}
	return c.dropCohortLinkRefIndexes[link-1]
}

// BindDropCohortLinkRefsOwned binds one finalized reference per exact graph
// link under the caller's scheduler transaction.
func (c *Core) BindDropCohortLinkRefsOwned(owner SchedulerTransactionToken, boundary ClassifiedBoundary, descriptor ActionRowDescriptor, selectedOrdinal int, plan ReductionPlan, links LinkRange, refs []DropCohortRef) error {
	if c == nil {
		return errors.New("parser-core phase zero: link bind on nil core")
	}
	return c.bindDropCohortLinkRefsOwned(owner, boundary, descriptor, selectedOrdinal, plan, links, refs)
}

// BindDropCohortLinkRefs binds one finalized reference per exact graph link
// inside an atomic scheduler transaction.
func (c *Core) BindDropCohortLinkRefs(boundary ClassifiedBoundary, descriptor ActionRowDescriptor, selectedOrdinal int, plan ReductionPlan, links LinkRange, refs []DropCohortRef) error {
	if c == nil {
		return errors.New("parser-core phase zero: link bind on nil core")
	}
	return c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		return c.BindDropCohortLinkRefsOwned(owner, boundary, descriptor, selectedOrdinal, plan, links, refs)
	})
}

type dropCohortLinkRefMutation struct {
	index    int
	previous uint32
}
