package lexexact

import "testing"

func TestMustHash(t *testing.T) {
	const h = "ad656555b6909bc8efbf40eceb2ac98121f7abdf707c829c9dc03ebe6d83bfbf"
	got := MustHash(h)
	if got[0] != 0xad || got[31] != 0xbf {
		t.Errorf("MustHash = %x", got)
	}
	for _, bad := range []string{"", "zz", h[:62]} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("MustHash(%q) did not panic", bad)
				}
			}()
			MustHash(bad)
		}()
	}
}
