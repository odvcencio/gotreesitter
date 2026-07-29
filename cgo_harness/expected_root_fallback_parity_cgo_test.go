//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestHurlExpectedRootFallbackLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "recovered_entry", source: "GET http://localhost:8000/hello\n\nxxx"},
		{name: "file_delimiter", source: "POST http://localhost:8000/data\nfile,"},
		{name: "clean", source: "GET http://localhost:8000/hello"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "hurl"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}

func TestIniExpectedRootFallbackLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{
			name:   "malformed_files",
			source: "[mypy]\nfiles =\n    one.py\n    two.py\nstrict = true\n",
		},
		{
			name:   "error_code_continuation",
			source: "[mypy]\nenable_error_code = \n    ignore-without-code,\n    unused-ignore\n",
		},
		{name: "clean", source: "[mypy]\nstrict = true\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "ini"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}
