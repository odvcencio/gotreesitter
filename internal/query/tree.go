package query

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strconv"
)

// TreeNode is what the tree digest records of one syntax node: the same facts
// that the harness's structural comparison (compareNodes) checks, so equal
// digests mean the two trees compare equal there.
type TreeNode struct {
	Depth      int
	Field      string
	Type       string
	Named      bool
	Missing    bool
	StartByte  uint32
	EndByte    uint32
	ChildCount int
}

// TreeHasher builds the digest of a syntax tree that a corpus case stores
// for its input. Add every node in pre-order; Field is the field name on the
// edge from the node's parent ("" for the root and for children without one).
type TreeHasher struct {
	h   hash.Hash
	buf []byte
}

// NewTreeHasher returns an empty tree digest.
func NewTreeHasher() *TreeHasher {
	return &TreeHasher{h: sha256.New()}
}

// Add records one node.
func (t *TreeHasher) Add(n TreeNode) {
	b := t.buf[:0]
	b = strconv.AppendInt(b, int64(n.Depth), 10)
	b = append(b, '\t')
	b = strconv.AppendQuote(b, n.Field)
	b = append(b, '\t')
	b = strconv.AppendQuote(b, n.Type)
	b = append(b, '\t')
	b = strconv.AppendBool(b, n.Named)
	b = append(b, '\t')
	b = strconv.AppendBool(b, n.Missing)
	b = append(b, '\t')
	b = strconv.AppendUint(b, uint64(n.StartByte), 10)
	b = append(b, '\t')
	b = strconv.AppendUint(b, uint64(n.EndByte), 10)
	b = append(b, '\t')
	b = strconv.AppendInt(b, int64(n.ChildCount), 10)
	b = append(b, '\n')
	t.h.Write(b)
	t.buf = b
}

// Sum returns the digest: "sha256:" and the first 16 hex digits.
func (t *TreeHasher) Sum() string {
	return "sha256:" + hex.EncodeToString(t.h.Sum(nil))[:16]
}
