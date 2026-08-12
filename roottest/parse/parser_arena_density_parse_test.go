package parse_test

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestArenaDensityLargeSourceDeepDigests(t *testing.T) {
	tests := []struct {
		name       string
		source     []byte
		language   func() *gotreesitter.Language
		wantCap    bool
		wantDigest string
	}{
		{
			name: "gap-aligned-dense-ascii", source: arenaDensityGapAlignedDenseGoSource(),
			language:   grammars.GoLanguage,
			wantDigest: "f691e4958fe13b8317702da2a47b7417af120511488820dd9beacff3a240f570",
		},
		{
			name:       "certified-odin-token-sparse-ascii",
			source:     arenaDensityOdinVectorSource(false),
			language:   grammars.OdinLanguage,
			wantCap:    true,
			wantDigest: "d460b2aa636ad2eb5b09f6686a43a23bec81298d2661a39ed55093e6424afed4",
		},
		{
			name:       "certified-odin-non-ascii-fail-open",
			source:     arenaDensityOdinVectorSource(true),
			language:   grammars.OdinLanguage,
			wantCap:    true,
			wantDigest: "5cedd647235d6f6a60cd554619aec7424f64b59e38df0d9bc07e3af0aba4a575",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.source) < 1<<20 {
				t.Fatalf("fixture is only %d bytes", len(test.source))
			}
			lang := test.language()
			if lang.FullParseArenaDensityCapEnabled != test.wantCap {
				t.Fatalf("FullParseArenaDensityCapEnabled=%t, want %t", lang.FullParseArenaDensityCapEnabled, test.wantCap)
			}
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root == nil || root.StartByte() != 0 || root.EndByte() != uint32(len(test.source)) || root.HasError() || tree.ParseStoppedEarly() {
				t.Fatalf("incomplete tree: root=%v source=%d stopped=%t runtime=%s", root, len(test.source), tree.ParseStoppedEarly(), tree.ParseRuntime().Summary())
			}
			inspection, err := benchfixtures.InspectGoTree(root, lang)
			if err != nil {
				t.Fatal(err)
			}
			if inspection.SHA256 != test.wantDigest {
				t.Fatalf("deep tree digest=%s, want baseline=%s", inspection.SHA256, test.wantDigest)
			}
		})
	}
}

func arenaDensityGapAlignedDenseGoSource() []byte {
	const (
		chunks      = 64
		chunkBytes  = 32 << 10
		sparseBytes = 16 << 10
	)
	var source bytes.Buffer
	source.Grow(chunks * chunkBytes)
	prefix := []byte("package p\n")
	source.Write(prefix)
	arenaDensityWriteExactComment(&source, sparseBytes-len(prefix))
	declaration := 0
	for chunk := 0; chunk < chunks; chunk++ {
		if chunk != 0 {
			arenaDensityWriteExactComment(&source, sparseBytes)
		}
		denseEnd := (chunk + 1) * chunkBytes
		for {
			line := []byte(fmt.Sprintf("\nvar v%06d = 1+2+3+4+5+6+7+8\n", declaration))
			if source.Len()+len(line) > denseEnd-4 {
				break
			}
			source.Write(line)
			declaration++
		}
		arenaDensityWriteExactComment(&source, denseEnd-source.Len())
	}
	if source.Len() != chunks*chunkBytes {
		panic(fmt.Sprintf("gap-aligned fixture bytes=%d want=%d", source.Len(), chunks*chunkBytes))
	}
	return source.Bytes()
}

func arenaDensityOdinVectorSource(nonASCII bool) []byte {
	var source bytes.Buffer
	source.WriteString("package arena_density\n\nvalues := []string{\n\t\"")
	source.Write(bytes.Repeat([]byte("x"), 2<<20))
	if nonASCII {
		source.WriteString("λ")
	}
	source.WriteString("\",\n}\n")
	return source.Bytes()
}

func arenaDensityWriteExactComment(dst *bytes.Buffer, size int) {
	if size < 4 {
		panic(fmt.Sprintf("comment size=%d is too small", size))
	}
	dst.WriteString("/*")
	dst.Write(bytes.Repeat([]byte("x"), size-4))
	dst.WriteString("*/")
}
