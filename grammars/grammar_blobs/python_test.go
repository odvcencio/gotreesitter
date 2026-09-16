package grammarblobs

import "testing"

func TestPythonReturnsIndependentBytes(t *testing.T) {
	first := Python()
	second := Python()
	if len(first) == 0 {
		t.Fatal("Python grammar blob is empty")
	}

	original := second[0]
	first[0] ^= 0xff
	if second[0] != original {
		t.Fatal("Python returned shared mutable storage")
	}
}
