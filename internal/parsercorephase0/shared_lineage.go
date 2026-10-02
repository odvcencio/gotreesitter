package parsercorephase0

import "errors"

type sharedLineageMutation struct {
	node      NodeID
	owner     uint32
	reference uint32
}

// EnableSharedLineageRecords stores scheduler ownership per node and interns
// the immutable history separately. Most nodes carry the same history; they
// need an index rather than another complete nodeLineageRecord.
func (c *Core) EnableSharedLineageRecords() error {
	if c == nil || len(c.nodes) != 0 || len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: shared lineage requires an empty core")
	}
	c.sharedLineages = true
	return nil
}

func (c *Core) nodeLineageValue(id NodeID) (nodeLineageRecord, error) {
	if !c.sharedLineages {
		record, err := c.nodeLineage(id)
		if err != nil {
			return nodeLineageRecord{}, err
		}
		return *record, nil
	}
	if id == 0 || uint64(id) > uint64(len(c.nodeLineageRefs)) {
		return nodeLineageRecord{}, errors.New("parser-core phase zero: invalid node lineage")
	}
	var record nodeLineageRecord
	if reference := c.nodeLineageRefs[id-1]; reference != 0 {
		if uint64(reference) > uint64(len(c.nodeLineages)) {
			return record, errors.New("parser-core phase zero: invalid shared lineage reference")
		}
		record = c.nodeLineages[reference-1]
	}
	record.owner = c.nodeOwners[id-1]
	return record, nil
}

func (c *Core) storeNodeLineage(id NodeID, record nodeLineageRecord) error {
	if !c.sharedLineages {
		c.nodeLineages[id-1] = record
		return nil
	}
	owner := record.owner
	record.owner = 0
	var reference uint32
	if record != (nodeLineageRecord{}) {
		if c.nodeLineageIntern == nil {
			c.nodeLineageIntern = make(map[nodeLineageRecord]uint32)
		}
		reference = c.nodeLineageIntern[record]
		if reference == 0 {
			if uint64(len(c.nodeLineages)) >= uint64(c.limits.MaxMetadata) {
				return errors.New("parser-core phase zero: shared lineage metadata cap")
			}
			c.nodeLineages = append(c.nodeLineages, record)
			reference = uint32(len(c.nodeLineages))
			c.nodeLineageIntern[record] = reference
			c.nodeLineageInternEntries = max(c.nodeLineageInternEntries, len(c.nodeLineageIntern))
		}
	}
	if c.nodeOwners[id-1] == owner && c.nodeLineageRefs[id-1] == reference {
		return nil
	}
	if len(c.transactions) != 0 {
		c.sharedLineageJournal = append(c.sharedLineageJournal, sharedLineageMutation{node: id, owner: c.nodeOwners[id-1], reference: c.nodeLineageRefs[id-1]})
	}
	c.nodeOwners[id-1], c.nodeLineageRefs[id-1] = owner, reference
	return nil
}

func (c *Core) restoreSharedLineages(mark *checkpoint) {
	for index := len(c.sharedLineageJournal) - 1; index >= mark.sharedLineageJournal; index-- {
		mutation := c.sharedLineageJournal[index]
		if uint64(mutation.node) <= uint64(mark.nodes) {
			c.nodeOwners[mutation.node-1] = mutation.owner
			c.nodeLineageRefs[mutation.node-1] = mutation.reference
		}
	}
	c.nodeOwners = c.nodeOwners[:mark.nodes]
	c.nodeLineageRefs = c.nodeLineageRefs[:mark.nodes]
	for _, record := range c.nodeLineages[mark.nodeLineages:] {
		delete(c.nodeLineageIntern, record)
	}
	c.sharedLineageJournal = c.sharedLineageJournal[:mark.sharedLineageJournal]
}

func (c *Core) reserveLineages(nodes int, growing bool) {
	if c.sharedLineages {
		if growing {
			c.nodeOwners = growArena(c.nodeOwners, nodes)
			c.nodeLineageRefs = growArena(c.nodeLineageRefs, nodes)
		} else {
			c.nodeOwners = reserveArena(c.nodeOwners, nodes)
			c.nodeLineageRefs = reserveArena(c.nodeLineageRefs, nodes)
		}
	} else if growing {
		c.nodeLineages = growArena(c.nodeLineages, nodes)
	} else {
		c.nodeLineages = reserveArena(c.nodeLineages, nodes)
	}
}

func (c *Core) reserveLineageBytes() uint64 {
	if c.sharedLineages {
		return 2 * coreUint32Bytes
	}
	return coreNodeLineageRecordBytes
}

func (c *Core) reserveTotalBytes(nodes, links, subtrees, children int) uint64 {
	return reserveTotalBytes(nodes, links, subtrees, children) - uint64(nodes)*(coreNodeLineageRecordBytes-c.reserveLineageBytes())
}

func (c *Core) releaseSharedLineages() {
	c.nodeOwners = nil
	c.nodeLineageRefs = nil
	c.nodeLineageIntern = nil
	c.nodeLineageInternEntries = 0
	c.sharedLineageJournal = nil
}

// NewWithSharedLineage creates the production candidate's compact graph.
// Diagnostic callers can keep the dense New representation.
func NewWithSharedLineage(tables TableView, limits Limits) (*Core, error) {
	compact, err := New(tables, limits)
	if err != nil {
		return nil, err
	}
	if err := compact.EnableSharedLineageRecords(); err != nil {
		return nil, err
	}
	return compact, nil
}
