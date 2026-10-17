package incr

// LeafReceipt stores the two high provenance bits of a native reuse word in
// two packed bits per subtree. An absent slot has no leaf authorization.
func LeafReceipt(words []uint32, id uint32) uint32 {
	index := uint64(id) / 16
	if index >= uint64(len(words)) {
		return 0
	}
	return (words[index] >> ((id & 15) * 2) & 3) << 30
}

// SetLeafReceipt replaces both provenance bits without changing neighboring
// subtrees. The producer must allocate the word before publishing a receipt.
func SetLeafReceipt(words []uint32, id uint32, flags uint32) {
	shift := (id & 15) * 2
	word := &words[uint64(id)/16]
	*word = *word&^(uint32(3)<<shift) | (flags>>30)<<shift
}

// TrimLeafReceiptTail revokes slots beyond the producer's last subtree ID.
// The caller has already resized words to include that ID and no later word.
func TrimLeafReceiptTail(words []uint32, last uint32) {
	words[len(words)-1] &= ^uint32(0) >> (30 - (last&15)*2)
}
