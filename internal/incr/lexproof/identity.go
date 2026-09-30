package lexproof

import (
	"encoding/binary"
	"reflect"
)

// Identity is an exact, bounded snapshot of a scanner's immutable parameters.
// Unsupported or wide values decline caching. No object or source is retained.
// Type distinguishes encodings whose field types and boundaries differ.
type Identity struct {
	Type   reflect.Type
	data   [256]byte
	length uint16
}

func Snapshot(value any) (Identity, bool) {
	id := Identity{Type: reflect.TypeOf(value)}
	if id.Type == nil {
		return id, true
	}
	steps := 256
	if !id.appendValue(reflect.ValueOf(value), 0, &steps) {
		return Identity{}, false
	}
	return id, true
}

func (id *Identity) appendUint(value uint64) bool {
	if int(id.length)+8 > len(id.data) {
		return false
	}
	binary.LittleEndian.PutUint64(id.data[id.length:], value)
	id.length += 8
	return true
}

func (id *Identity) appendValue(v reflect.Value, depth int, steps *int) bool {
	if depth > 16 || *steps == 0 {
		return false
	}
	*steps -= 1
	switch v.Kind() {
	case reflect.Bool:
		value := uint64(0)
		if v.Bool() {
			value = 1
		}
		return id.appendUint(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return id.appendUint(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return id.appendUint(v.Uint())
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !id.appendValue(v.Index(i), depth+1, steps) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !id.appendValue(v.Field(i), depth+1, steps) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if !id.appendUint(uint64(v.Len())) || !id.appendUint(uint64(v.Cap())) || !id.appendUint(uint64(v.Pointer())) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !id.appendValue(v.Index(i), depth+1, steps) {
				return false
			}
		}
		return true
	case reflect.String:
		s := v.String()
		if !id.appendUint(uint64(len(s))) || len(s) > len(id.data)-int(id.length) {
			return false
		}
		copy(id.data[id.length:], s)
		id.length += uint16(len(s))
		return true
	case reflect.Pointer:
		if v.IsNil() {
			return id.appendUint(0)
		}
		if !id.appendUint(uint64(v.Pointer())) {
			return false
		}
		return id.appendValue(v.Elem(), depth+1, steps)
	default:
		return false
	}
}
