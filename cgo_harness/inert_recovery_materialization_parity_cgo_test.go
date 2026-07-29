//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestAngularInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runInertRecoveryMaterializationParity(t, "angular", []inertRecoveryMaterializationParityCase{
		{name: "let_non_null", source: "<div>@let itemLabel = item.label!;</div>"},
		{name: "binary_non_null", source: "<div>@if (item.level! > 1) { ok }</div>"},
		{name: "ampersand_text", source: "<strong>Opinionated & versatile,</strong>"},
	})
}

func TestBibtexInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runInertRecoveryMaterializationParity(t, "bibtex", []inertRecoveryMaterializationParityCase{
		{
			name:   "missing_comma_after_key",
			source: "@article{test\n    title     = {title-from-embedded-bibtex-file},\n    author    = {author-from-embedded-bibtex-file},\n}\n",
		},
		{
			name:   "missing_key",
			source: "@techreport{\n  journal = {Wirtschaftsinformatik},\n  author = {Sam and jason},\n}\n",
		},
	})
}

func TestEDSInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runInertRecoveryMaterializationParity(t, "eds", []inertRecoveryMaterializationParityCase{
		{name: "embedded_section", source: "[A]\nEmpty=\n\n[B]\nX=1\n"},
		{
			name:   "error_lines",
			source: "[1003sub0]\nParameterName=Number of errors\nObjectType=0x7\n;StorageLocation=RAM\nDataType=0x0005\nAccessType=rw\nDefaultValue=\nPDOMapping=0\n",
		},
	})
}

func TestChatitoInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runInertRecoveryMaterializationParity(t, "chatito", []inertRecoveryMaterializationParityCase{
		{name: "trailing_alias_word", source: "~[minute]\n    58\n    59"},
	})
}

type inertRecoveryMaterializationParityCase struct {
	name   string
	source string
}

func runInertRecoveryMaterializationParity(
	t *testing.T,
	language string,
	tests []inertRecoveryMaterializationParityCase,
) {
	t.Helper()
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: language},
				test.name,
				[]byte(test.source),
			)
		})
	}
}
