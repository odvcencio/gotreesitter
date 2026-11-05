package parsercorephase0

// NativeProjectionDirectFieldsSupported reports whether every raw production
// published in this generation has the direct, single-field-per-child profile
// supported by streaming publication. Rejected speculative productions may
// conservatively keep this false until Reset; they never authorize a publisher.
func (c *Core) NativeProjectionDirectFieldsSupported() bool {
	return c != nil && !c.nativeProjectionFieldProfileUnsupported
}

func nativeProjectionDirectSingleFields(fields []FieldMapEntry) bool {
	var seen [4]uint64
	for _, field := range fields {
		if field.Inherited {
			return false
		}
		slot, bit := field.ChildIndex/64, uint64(1)<<(field.ChildIndex%64)
		if seen[slot]&bit != 0 {
			return false
		}
		seen[slot] |= bit
	}
	return true
}
