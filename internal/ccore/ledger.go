package ccore

import (
	"math"
	"slices"
	"unsafe"
)

// Ledger is an optional accounting primitive for a single request. It is not
// connected to the parser until callers explicitly use it for their allocations.
// It counts caller-supplied starting capacity and requested backing-array or slab
// bytes, including replaced arrays. It does not count Go allocator size-class
// rounding, runtime overhead, or allocations that do not pass through it.
//
// Ordinary reservations poll when pending charges reach LedgerGranularity or a
// single charge is at least that large. A refused reservation changes no state;
// a failed poll or invalid charge ends the request. Force is for mandatory
// cleanup and can exceed that polling granularity and the budget before the next
// check. A poll must decide whether its supplied total is within the limit.
// With no poll, helpers use the standard allocation path for valid sizes and do
// not account their allocations. Direct Reserve calls still update Alloc.

// LedgerGranularity is the ordinary reservation polling threshold: 256 KiB.
// Force charges are not subject to this threshold.
const LedgerGranularity = 256 << 10

// Ledger is the account of one request. The zero value has no limit. Base and
// Alloc are readable counters and must not be changed directly by callers.
type Ledger struct {
	Base     int64 // capacity the request held when it began (pooled parser, then pooled arena)
	Alloc    int64 // charged array and slab bytes since Begin, except reservations moved to Base
	pending  int64 // unpolled reserved bytes, or LedgerGranularity after forced cleanup
	overflow bool  // forced cleanup exceeded representable accounting
	poll     func(total int64, sample bool) bool
}

// SetPoll installs the limit check, or removes it (nil). The check gets the
// charged total (the allocation about to be made included) and whether a real
// heap sample is asked for; it returns false to refuse. Configure it before Begin
// and keep it unchanged until that request and its mandatory cleanup end.
func (l *Ledger) SetPoll(f func(total int64, sample bool) bool) { l.poll = f }

// Active reports whether a limit check is installed.
func (l *Ledger) Active() bool { return l.poll != nil }

// Begin starts an account: Base is the capacity the request holds, Alloc and the
// pending bytes are zero. It makes the first limit check, so a request that
// starts above its budget stops before any work. A negative base is refused.
// It returns true with no limit for a valid base.
func (l *Ledger) Begin(base int64) bool {
	if base < 0 {
		return false
	}
	l.Base, l.Alloc, l.pending, l.overflow = base, 0, 0, false
	return l.Check(false)
}

// Force counts n bytes that were made, or are about to be, by work that cannot be
// refused (reclamation: freeing records must finish, or the store would keep
// records nobody can reach). It never fails and does not ask the limit; the
// next reservation polls after a positive forced charge and can stop the request.
// Invalid or overflowing charges saturate the total and make future checks fail
// until Begin resets the account; mandatory cleanup itself must still finish.
func (l *Ledger) Force(n int64) {
	if l == nil {
		return
	}
	if n < 0 || n > math.MaxInt64-l.Base-l.Alloc {
		l.Alloc = math.MaxInt64 - l.Base
		l.overflow = true
		return
	}
	if l.poll != nil {
		l.Alloc += n
		if n > 0 {
			l.pending = LedgerGranularity
		}
	}
}

// Total is the charged total: Base + Alloc.
func (l *Ledger) Total() int64 { return l.Base + l.Alloc }

// Check asks the limit now. sample asks for a real heap sample.
func (l *Ledger) Check(sample bool) bool {
	if l.overflow {
		return false
	}
	if l.poll != nil && !l.poll(l.Base+l.Alloc, sample) {
		return false
	}
	l.pending = 0
	return true
}

// Rebase replaces a reservation of reserved bytes by held bytes that the request
// really holds now (the cold arena: the size of the reservation is the most the
// arena can need, the arena then tells what it made). It checks the limit again
// when it raised the total. A refusal still records the actual held bytes;
// the caller must stop the request.
func (l *Ledger) Rebase(reserved, held int64) bool {
	if l.overflow || reserved < 0 || held < 0 || reserved > l.Alloc {
		return false
	}
	if held > math.MaxInt64-l.Base-(l.Alloc-reserved) {
		l.Alloc = math.MaxInt64 - l.Base
		l.overflow = true
		return false
	}
	l.Alloc -= reserved
	l.Base += held
	if held > reserved {
		return l.Check(false)
	}
	return true
}

// reservationResult distinguishes invalid accounting from a refused poll.
type reservationResult uint8

const (
	reservationOK reservationResult = iota
	reservationInvalid
	reservationPollRefused
)

func (l *Ledger) reserveClassified(n int64) reservationResult {
	if l.overflow || n < 0 || n > math.MaxInt64-l.Base-l.Alloc {
		return reservationInvalid
	}
	if l.poll != nil {
		if n >= LedgerGranularity || l.pending >= LedgerGranularity-n {
			if !l.poll(l.Base+l.Alloc+n, false) {
				return reservationPollRefused
			}
			l.pending = 0
		} else {
			l.pending += n
		}
	}
	l.Alloc += n // the only statement that counts a new array
	return reservationOK
}

// Reserve is called before n bytes are made. It returns false when the limit
// check refuses or n is invalid or overflows; the bytes are then not counted
// and must not be allocated. A false result stops the current request.
func (l *Ledger) Reserve(n int64) bool { return l.reserveClassified(n) == reservationOK }

func sizeOf[T any]() int64 {
	var z T
	return int64(unsafe.Sizeof(z))
}

// Grow returns s with room for extra more elements, in a new array when it has
// none. The new capacity is max(2*cap(s), len(s)+extra, 8); the whole new array
// is reserved before it is made. It returns false, with s unchanged, when the
// reservation is refused. With no limit active it is slices.Grow.
func Grow[T any](l *Ledger, s []T, extra int) ([]T, bool) {
	out, reason := growClassified(l, s, extra)
	return out, reason == reservationOK
}
func growClassified[T any](l *Ledger, s []T, extra int) ([]T, reservationResult) {
	if l.overflow {
		return s, reservationInvalid
	}
	n, ok := GrowSize(s, extra)
	if !ok {
		return s, reservationInvalid
	}
	if n == 0 {
		return s, reservationOK
	}
	if l.poll == nil {
		return slices.Grow(s, extra), reservationOK
	}
	bytes, ok := sliceBytes[T](n)
	if !ok {
		return s, reservationInvalid
	}
	if reason := l.reserveClassified(bytes); reason != reservationOK {
		return s, reason
	}
	t := make([]T, len(s), n)
	copy(t, s)
	return t, reservationOK
}

// GrowSize returns the required capacity and false if the size is invalid or
// cannot be represented. A zero capacity means the existing slice has room.
func GrowSize[T any](s []T, extra int) (int, bool) {
	return growthSize(len(s), cap(s), extra, sizeOf[T]())
}

// byteLimit bounds accounting and machine-addressable allocation sizes.
// The runtime may reject smaller allocations for other reasons.
func byteLimit() int64 {
	maxAddr := uint64(^uintptr(0))
	if maxAddr < uint64(math.MaxInt64) {
		return int64(maxAddr)
	}
	return math.MaxInt64
}

// growthSize checks lengths and machine-addressable byte sizes.
func growthSize(length, capacity, extra int, elemBytes int64) (int, bool) {
	if length < 0 || capacity < length || extra < 0 || extra > math.MaxInt-length || elemBytes < 0 {
		return 0, false
	}
	needed := length + extra
	if needed <= capacity {
		return 0, true
	}
	maxElements := math.MaxInt
	if elemBytes != 0 {
		if q := byteLimit() / elemBytes; q < int64(math.MaxInt) {
			maxElements = int(q)
		}
	}
	if needed > maxElements {
		return 0, false
	}
	doubled := maxElements
	if capacity <= maxElements/2 {
		doubled = capacity * 2
	}
	n := max(doubled, needed, min(8, maxElements))
	return n, true
}

func sliceBytes[T any](n int) (int64, bool) {
	if n < 0 {
		return 0, false
	}
	size := sizeOf[T]()
	if size != 0 && int64(n) > byteLimit()/size {
		return 0, false
	}
	return int64(n) * size, true
}

// GrowTo returns s in a new array of capacity n (n from GrowSize). The caller has
// reserved n elements; this is how several arrays are reserved with one check.
// When n is zero, no growth is needed and s is returned unchanged.
func GrowTo[T any](s []T, n int) []T {
	if n == 0 {
		return s
	}
	t := make([]T, len(s), n)
	copy(t, s)
	return t
}

// ElemBytes is the size of one element of T.
func ElemBytes[T any]() int64 { return sizeOf[T]() }

// Make returns a zeroed slice of n elements; its bytes are reserved first.
func Make[T any](l *Ledger, n int) ([]T, bool) {
	s, reason := makeClassified[T](l, n, false)
	return s, reason == reservationOK
}

// MakeCap is Make with a length of 0 and a capacity of n.
func MakeCap[T any](l *Ledger, n int) ([]T, bool) {
	s, reason := makeClassified[T](l, n, true)
	return s, reason == reservationOK
}
func makeClassified[T any](l *Ledger, n int, capacityOnly bool) ([]T, reservationResult) {
	if l.overflow {
		return nil, reservationInvalid
	}
	bytes, ok := sliceBytes[T](n)
	if !ok {
		return nil, reservationInvalid
	}
	if l.poll != nil {
		if reason := l.reserveClassified(bytes); reason != reservationOK {
			return nil, reason
		}
	}
	if capacityOnly {
		return make([]T, 0, n), reservationOK
	}
	return make([]T, n), reservationOK
}

// Push appends v to s. A full slice grows through Grow (the reservation is the
// check). It returns false, with s unchanged, when the reservation is refused.
func Push[T any](l *Ledger, s []T, v ...T) ([]T, bool) {
	out, reason := pushClassified(l, s, v...)
	return out, reason == reservationOK
}
func pushClassified[T any](l *Ledger, s []T, v ...T) ([]T, reservationResult) {
	if l.overflow || len(v) > math.MaxInt-len(s) {
		return s, reservationInvalid
	}
	if len(v) > cap(s)-len(s) {
		if _, ok := sliceBytes[T](len(s) + len(v)); !ok {
			return s, reservationInvalid
		}
	}
	if l.poll != nil && len(v) > cap(s)-len(s) {
		var reason reservationResult
		s, reason = growClassified(l, s, len(v))
		if reason != reservationOK {
			return s, reason
		}
	}
	return append(s, v...), reservationOK
}

// ledgerStop is the one value the core panics with when the ledger refuses a
// reservation in code that has no error path. A future caller that uses these
// helpers must recover this type at its request boundary.
type ledgerStop struct{ reason reservationResult }

// push, grow, mk and mkCap are the core's helpers: a refused reservation unwinds by
// a panic of ledgerStop. Nothing was allocated or changed by the refused call.
func push[T any](l *Ledger, s []T, v ...T) []T {
	out, reason := pushClassified(l, s, v...)
	if reason != reservationOK {
		panic(ledgerStop{reason: reason})
	}
	return out
}
func grow[T any](l *Ledger, s []T, extra int) []T {
	out, reason := growClassified(l, s, extra)
	if reason != reservationOK {
		panic(ledgerStop{reason: reason})
	}
	return out
}
func mk[T any](l *Ledger, n int) []T {
	out, reason := makeClassified[T](l, n, false)
	if reason != reservationOK {
		panic(ledgerStop{reason: reason})
	}
	return out
}
func mkCap[T any](l *Ledger, n int) []T {
	out, reason := makeClassified[T](l, n, true)
	if reason != reservationOK {
		panic(ledgerStop{reason: reason})
	}
	return out
}

// deleteAt removes s[i] by moving the tail down (no allocation).
func deleteAt[T any](s []T, i int) []T {
	copy(s[i:], s[i+1:])
	return s[:len(s)-1]
}

// fpush charges growth as mandatory cleanup. If the required capacity or byte
// count cannot be represented, it marks the ledger overflowed and panics: an
// impossible Go slice cannot be allocated to finish cleanup. No invalid make is
// attempted. A nil ledger means no request is being accounted.
func fpush[T any](l *Ledger, s []T, v ...T) []T {
	if len(v) > cap(s)-len(s) {
		n, ok := GrowSize(s, len(v))
		if !ok {
			l.Force(-1)
			panic("ccore: forced cleanup size is unrepresentable")
		}
		bytes, ok := sliceBytes[T](n)
		if !ok {
			l.Force(-1)
			panic("ccore: forced cleanup size is unrepresentable")
		}
		l.Force(bytes)
		t := make([]T, len(s), n)
		copy(t, s)
		s = t
	}
	return append(s, v...)
}

// fmake charges mandatory cleanup. Unrepresentable sizes mark overflow and
// panic before make; Go cannot allocate such a slice to complete cleanup.
func fmake[T any](l *Ledger, n int) []T {
	bytes, ok := sliceBytes[T](n)
	if !ok {
		l.Force(-1)
		panic("ccore: forced cleanup size is unrepresentable")
	}
	l.Force(bytes)
	return make([]T, n)
}
