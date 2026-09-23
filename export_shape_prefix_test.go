package gotreesitter

// SetGSSShapePrefixVerifyForTest turns on the shape-prefix cache oracle
// (gssShapePrefixVerify) and resets its mismatch counter. It returns a
// restore function. Tests in the external package use it to prove the
// conditional invalidation never serves a stale prefix.
func SetGSSShapePrefixVerifyForTest() func() {
	gssShapePrefixVerify = true
	gssShapePrefixVerifyMismatches.Store(0)
	return func() { gssShapePrefixVerify = false }
}

// GSSShapePrefixVerifyMismatchesForTest reports how many cache hits the
// oracle contradicted since the last SetGSSShapePrefixVerifyForTest.
func GSSShapePrefixVerifyMismatchesForTest() uint64 {
	return gssShapePrefixVerifyMismatches.Load()
}
