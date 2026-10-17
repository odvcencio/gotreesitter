package parsercorephase0

// validateShape accepts an ordinary payload with either valid order flag.
// Scores and order values keep their full widths, including an unflagged
// nonzero order. The original validator handles recovery and malformed input
// in the original error order.
func (l linkRecord) validateShape() error {
	if l.payload != 0 && l.flags & ^linkFlagHasOrder == 0 {
		return nil
	}
	return l.validateUncommonShape()
}
