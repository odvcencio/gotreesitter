package parsercorephase0

import "unsafe"

const projectionFirstSlots = 256
const projectionPageSlots = 1024

// ProjectionPages holds private construction records at stable indexed slots.
// A small first page avoids over-reserving short trees. Later pages do not
// copy earlier records, and Clear releases their pointers before pool reuse.
// The owner includes the value itself in its own object-size accounting.
type ProjectionPages[T any] struct {
	first  *[projectionFirstSlots]T
	pages  []*[projectionPageSlots]T
	length int
}

func (p *ProjectionPages[T]) Len() int { return p.length }

func (p *ProjectionPages[T]) Append(value T) int {
	index := p.length
	if index < projectionFirstSlots {
		if p.first == nil {
			p.first = new([projectionFirstSlots]T)
		}
		p.first[index] = value
	} else {
		n := index - projectionFirstSlots
		page := n / projectionPageSlots
		if page == len(p.pages) {
			p.pages = append(p.pages, new([projectionPageSlots]T))
		}
		p.pages[page][n%projectionPageSlots] = value
	}
	p.length++
	return index
}

func (p *ProjectionPages[T]) At(index int) *T {
	if index < 0 || index >= p.length {
		panic("parser-core phase zero: projection slot out of range")
	}
	if index < projectionFirstSlots {
		return &p.first[index]
	}
	n := index - projectionFirstSlots
	return &p.pages[n/projectionPageSlots][n%projectionPageSlots]
}

func (p *ProjectionPages[T]) Clear() {
	if p.first != nil {
		clear(p.first[:])
	}
	for _, page := range p.pages {
		clear(page[:])
	}
	p.length = 0
}

// FootprintBytes charges every allocated slot, including unused retained pages.
func (p *ProjectionPages[T]) FootprintBytes() uint64 {
	total := uint64(cap(p.pages)) * uint64(unsafe.Sizeof((*[projectionPageSlots]T)(nil)))
	if p.first != nil {
		total += uint64(unsafe.Sizeof(*p.first))
	}
	return total + uint64(len(p.pages))*uint64(unsafe.Sizeof([projectionPageSlots]T{}))
}
