package gotreesitter

import "unsafe"

// nodeFieldMetadata keeps the uncommon child-field slice headers out of Node.
// The backing slices retain their existing ownership and aliasing semantics;
// replacing either header allocates a new sidecar so shallow Node copies keep
// independent headers while continuing to share backing storage.
type nodeFieldMetadata struct {
	ids     []FieldID
	sources []uint8
}

type nodeFieldMetadataSlab struct {
	data []nodeFieldMetadata
	used int
}

func (n *Node) fieldIDs() []FieldID {
	if n == nil || n.fieldMetadata == nil {
		return nil
	}
	return n.fieldMetadata.ids
}

func (n *Node) fieldSources() []uint8 {
	if n == nil || n.fieldMetadata == nil {
		return nil
	}
	return n.fieldMetadata.sources
}

func (n *Node) setFieldMetadata(ids []FieldID, sources []uint8) {
	if n == nil {
		return
	}
	if ids == nil && sources == nil {
		n.fieldMetadata = nil
		return
	}
	metadata := n.ownerArena.allocNodeFieldMetadata()
	metadata.ids = ids
	metadata.sources = sources
	n.fieldMetadata = metadata
}

func (n *Node) setFieldIDs(ids []FieldID) {
	if n == nil {
		return
	}
	n.setFieldMetadata(ids, n.fieldSources())
}

func (n *Node) setFieldSources(sources []uint8) {
	if n == nil {
		return
	}
	n.setFieldMetadata(n.fieldIDs(), sources)
}

func (n *Node) clearFieldMetadata() {
	if n != nil {
		n.fieldMetadata = nil
	}
}

func equalFieldIDSlices(a, b []FieldID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizedFieldSourceForID(fieldIDs []FieldID, fieldSources []uint8, i int) uint8 {
	if i < 0 || i >= len(fieldIDs) || fieldIDs[i] == 0 {
		return fieldSourceNone
	}
	source := fieldSourceAt(fieldSources, i)
	if source == fieldSourceNone {
		return fieldSourceDirect
	}
	return source
}

func equalNodeFieldMetadata(a, b *Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	aIDs := a.fieldIDs()
	bIDs := b.fieldIDs()
	if !equalFieldIDSlices(aIDs, bIDs) {
		return false
	}
	aSources := a.fieldSources()
	bSources := b.fieldSources()
	for i := range aIDs {
		if normalizedFieldSourceForID(aIDs, aSources, i) != normalizedFieldSourceForID(bIDs, bSources, i) {
			return false
		}
	}
	return true
}

func cloneNodeFieldMetadataHeaderInto(dst, src *Node, arena *nodeArena) {
	if dst == nil || src == nil || src.fieldMetadata == nil {
		if dst != nil {
			dst.fieldMetadata = nil
		}
		return
	}
	metadata := arena.allocNodeFieldMetadata()
	metadata.ids = src.fieldMetadata.ids
	metadata.sources = src.fieldMetadata.sources
	dst.fieldMetadata = metadata
}

func (a *nodeArena) allocNodeFieldMetadata() *nodeFieldMetadata {
	if a == nil {
		return &nodeFieldMetadata{}
	}
	if len(a.nodeFieldMetadataSlabs) == 0 {
		capacity := defaultNodeFieldMetadataSlabCap(a.class)
		a.nodeFieldMetadataSlabs = append(a.nodeFieldMetadataSlabs, nodeFieldMetadataSlab{
			data: make([]nodeFieldMetadata, capacity),
		})
		a.allocatedBytes += nodeFieldMetadataBytesForCap(capacity)
		a.nodeFieldMetadataSlabCursor = 0
	}
	if a.nodeFieldMetadataSlabCursor < 0 || a.nodeFieldMetadataSlabCursor >= len(a.nodeFieldMetadataSlabs) {
		a.nodeFieldMetadataSlabCursor = 0
	}
	for i := a.nodeFieldMetadataSlabCursor; ; i++ {
		if i >= len(a.nodeFieldMetadataSlabs) {
			lastCap := len(a.nodeFieldMetadataSlabs[len(a.nodeFieldMetadataSlabs)-1].data)
			capacity := boundedNextSlabCap(lastCap, minArenaNodeCap, int(unsafe.Sizeof(nodeFieldMetadata{})))
			a.nodeFieldMetadataSlabs = append(a.nodeFieldMetadataSlabs, nodeFieldMetadataSlab{
				data: make([]nodeFieldMetadata, capacity),
			})
			a.allocatedBytes += nodeFieldMetadataBytesForCap(capacity)
		}

		slab := &a.nodeFieldMetadataSlabs[i]
		if slab.used >= len(slab.data) {
			continue
		}
		idx := slab.used
		slab.used++
		a.nodeFieldMetadataSlabCursor = i
		return &slab.data[idx]
	}
}

func (a *nodeArena) resetNodeFieldMetadataSlabs() {
	if len(a.nodeFieldMetadataSlabs) > 0 {
		retained := 0
		keep := 0
		limit := maxRetainedNodeFieldMetadataCapacityForClass(a.class)
		for i := 0; i < len(a.nodeFieldMetadataSlabs); i++ {
			capacity := len(a.nodeFieldMetadataSlabs[i].data)
			if capacity <= 0 || retained+capacity > limit {
				break
			}
			retained += capacity
			keep = i + 1
		}
		for i := keep; i < len(a.nodeFieldMetadataSlabs); i++ {
			a.nodeFieldMetadataSlabs[i] = nodeFieldMetadataSlab{}
		}
		a.nodeFieldMetadataSlabs = a.nodeFieldMetadataSlabs[:keep]
	}
	for i := range a.nodeFieldMetadataSlabs {
		slab := &a.nodeFieldMetadataSlabs[i]
		used := min(slab.used, len(slab.data))
		clear(slab.data[:used])
		slab.used = 0
	}
	a.nodeFieldMetadataSlabCursor = 0
}

func defaultNodeFieldMetadataSlabCap(class arenaClass) int {
	return max(nodeCapacityForClass(class)/8, minArenaNodeCap)
}

func maxRetainedNodeFieldMetadataCapacityForClass(class arenaClass) int {
	size := int(unsafe.Sizeof(nodeFieldMetadata{}))
	if size <= 0 {
		size = 1
	}
	if class == arenaClassFull {
		return max(maxRetainedFullNodeBytes/size, defaultNodeFieldMetadataSlabCap(class))
	}
	floor := maxRetainedIncrementalNodeBytes / size
	return max(defaultNodeFieldMetadataSlabCap(class)*maxRetainedArenaFactor, floor)
}

func nodeFieldMetadataBytesForCap(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * int64(unsafe.Sizeof(nodeFieldMetadata{}))
}

func (a *nodeArena) nodeFieldMetadataBytesAllocated() int64 {
	if a == nil {
		return 0
	}
	var total int64
	for i := range a.nodeFieldMetadataSlabs {
		total += nodeFieldMetadataBytesForCap(len(a.nodeFieldMetadataSlabs[i].data))
	}
	return total
}
