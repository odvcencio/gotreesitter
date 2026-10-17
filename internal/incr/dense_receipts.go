package incr

import (
	"sort"
	"unsafe"
)

// DenseReceipt stores one coordinate-authenticated dependency without node
// pointers. Presence distinguishes a valid zero extent from missing proof.
type DenseReceipt struct {
	Start, End, Lookahead, Column uint32
	Present                       bool
}

// DenseReceipts allocates pages only for node ranges that receive receipts.
// Storage owners serialize writes and may read immutable pages concurrently.
type DenseReceipts struct {
	pages  []*denseReceiptPage
	sparse []sparseDenseReceipt
	dense  bool
	count  int
}

type denseReceiptPage [64]DenseReceipt
type sparseDenseReceipt struct {
	index   int
	receipt DenseReceipt
}

func (d *DenseReceipts) Count() int { return d.count }
func (d *DenseReceipts) Bytes() int64 {
	bytes := int64(cap(d.sparse))*int64(unsafe.Sizeof(sparseDenseReceipt{})) + int64(cap(d.pages))*int64(unsafe.Sizeof((*denseReceiptPage)(nil)))
	for _, p := range d.pages {
		if p != nil {
			bytes += int64(unsafe.Sizeof(denseReceiptPage{}))
		}
	}
	return bytes
}
func (d *DenseReceipts) sparsePosition(index int) (int, bool) {
	position := sort.Search(len(d.sparse), func(i int) bool { return d.sparse[i].index >= index })
	return position, position < len(d.sparse) && d.sparse[position].index == index
}
func (d *DenseReceipts) Lookup(index int) *DenseReceipt {
	if d == nil || index < 0 {
		return nil
	}
	if !d.dense {
		position, ok := d.sparsePosition(index)
		if !ok {
			return nil
		}
		return &d.sparse[position].receipt
	}
	if index/64 >= len(d.pages) || d.pages[index/64] == nil {
		return nil
	}
	return &d.pages[index/64][index%64]
}

// Ensure allocates a group of adjacent pages when the storage owner can
// fund it. A small budget falls back to one page without widening admission.
func (d *DenseReceipts) Ensure(index int, allow func(int64) bool) *DenseReceipt {
	if index < 0 {
		return nil
	}
	if d.dense {
		return d.ensureDense(index, allow)
	}
	position, exists := d.sparsePosition(index)
	if exists {
		return &d.sparse[position].receipt
	}
	high := index
	if len(d.sparse) > 0 && d.sparse[len(d.sparse)-1].index > high {
		high = d.sparse[len(d.sparse)-1].index
	}
	pages := (high / 64) + 1
	// Promote only once enough live receipts pay for all dense capacity.
	denseBytes := int64(pages)*int64(unsafe.Sizeof(denseReceiptPage{})) + int64(pages)*int64(unsafe.Sizeof((*denseReceiptPage)(nil)))
	if len(d.sparse) >= 64 && denseBytes <= int64(d.count+1)*int64(unsafe.Sizeof(sparseDenseReceipt{}))*2 && allow(denseBytes) {
		items := make([]denseReceiptPage, pages)
		d.pages = make([]*denseReceiptPage, pages)
		for i := range items {
			d.pages[i] = &items[i]
		}
		for _, entry := range d.sparse {
			d.pages[entry.index/64][entry.index%64] = entry.receipt
		}
		// Keep previously charged sparse capacity explicit until reset.
		clear(d.sparse)
		d.sparse = d.sparse[:0]
		d.dense = true
		return &d.pages[index/64][index%64]
	}
	if len(d.sparse) == cap(d.sparse) {
		capacity := max(4, 2*cap(d.sparse))
		cost := int64(capacity-cap(d.sparse)) * int64(unsafe.Sizeof(sparseDenseReceipt{}))
		if !allow(cost) {
			return nil
		}
		grown := make([]sparseDenseReceipt, len(d.sparse), capacity)
		copy(grown, d.sparse)
		d.sparse = grown
	}
	d.sparse = append(d.sparse, sparseDenseReceipt{})
	copy(d.sparse[position+1:], d.sparse[position:len(d.sparse)-1])
	d.sparse[position] = sparseDenseReceipt{index: index}
	return &d.sparse[position].receipt
}
func (d *DenseReceipts) ensureDense(index int, allow func(int64) bool) *DenseReceipt {
	page := index / 64
	if slot := d.Lookup(index); slot != nil {
		return slot
	}
	first := page &^ 7
	end := first + 8
	newPages := 0
	for i := first; i < end; i++ {
		if i >= len(d.pages) || d.pages[i] == nil {
			newPages++
		}
	}
	capacity := max(cap(d.pages), end)
	cost := int64(capacity-cap(d.pages))*int64(unsafe.Sizeof((*denseReceiptPage)(nil))) + int64(newPages)*int64(unsafe.Sizeof(denseReceiptPage{}))
	if !allow(cost) {
		first, end, newPages = page, page+1, 1
		capacity = max(cap(d.pages), end)
		cost = int64(capacity-cap(d.pages))*int64(unsafe.Sizeof((*denseReceiptPage)(nil))) + int64(unsafe.Sizeof(denseReceiptPage{}))
		if !allow(cost) {
			return nil
		}
	}
	if capacity > cap(d.pages) {
		grown := make([]*denseReceiptPage, capacity)
		copy(grown, d.pages)
		d.pages = grown
	} else if end > len(d.pages) {
		d.pages = d.pages[:end]
	}
	group := make([]denseReceiptPage, newPages)
	next := 0
	for i := first; i < end; i++ {
		if d.pages[i] == nil {
			d.pages[i] = &group[next]
			next++
		}
	}
	return &d.pages[page][index%64]
}

func (d *DenseReceipts) Set(index int, receipt DenseReceipt) {
	slot := d.Lookup(index)
	if !slot.Present {
		d.count++
	}
	*slot = receipt
	slot.Present = true
}
func (d *DenseReceipts) Clear(index int) {
	slot := d.Lookup(index)
	if slot != nil && slot.Present {
		*slot = DenseReceipt{}
		d.count--
	}
}
func (d *DenseReceipts) Each(visit func(int, DenseReceipt)) {
	if !d.dense {
		for _, entry := range d.sparse {
			if entry.receipt.Present {
				visit(entry.index, entry.receipt)
			}
		}
		return
	}
	for page, items := range d.pages {
		if items == nil {
			continue
		}
		for i, receipt := range items {
			if receipt.Present {
				visit(page*64+i, receipt)
			}
		}
	}
}
