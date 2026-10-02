//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestPerlDeclaredShiftLockedCParity(t *testing.T) {
	for _, source := range []string{
		"{l$s, }", "{f$s, }", "{l$s,$t}", "{l$s,$t, }",
		"{(p())if@;}", "{(f())if@a;}", "{(f($x))if@a;}", "{(f(1,2))if@a;}", "{(p())unless@a;}",
		"push @found, $_;\n", "push @found, $a, $b;\n",
		"{a(())if@_}",
		"opmask_add(invert_opset opset(@_)) if @_;\n",
		"opmask_add(opset(@_)) if @_;\n",
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "perl", candidateRoute: &candidate}, "declared-shift", []byte(source))
			}
		})
	}
}
