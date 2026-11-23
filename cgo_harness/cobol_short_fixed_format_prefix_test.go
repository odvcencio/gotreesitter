//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"
)

func TestCobolShortFixedFormatPrefix(t *testing.T) {
	const program = "       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. EXAMPLE.\n" +
		"       PROCEDURE DIVISION.\n" +
		"           DISPLAY 'HELLO'.\n" +
		"           STOP RUN.\n"

	lang, oracle := ccoreLanguages(t, "cobol")
	core := ccoreNew(lang)
	t.Run("leading", func(t *testing.T) {
		for _, newline := range []struct {
			name string
			text string
			max  int
		}{{"LF", "\n", 6}, {"CRLF", "\r\n", 2}} {
			for width := 1; width <= newline.max; width++ {
				t.Run(fmt.Sprintf("%s_%d", newline.name, width), func(t *testing.T) {
					src := []byte("ABCDEF"[:width] + newline.text + program)
					ref := compactT3ParseC(t, oracle, src)
					defer ref.Close()
					if stop := core.Parse(src, nil); stop != "" {
						t.Fatalf("ccore stop: %q", stop)
					}
					defer core.ReleaseLast()
					if got, want := core.Digest(), ccoreCDigest(ref); got != want {
						t.Fatalf("ccore digest differs from pinned C: %s != %s", got, want)
					}
				})
			}
		}
	})
	t.Run("trailing", func(t *testing.T) {
		// The pinned scanner can spin at EOF on these partial lines.
		for _, suffix := range []string{"", "\n"} {
			for width := 1; width <= 5; width++ {
				src := []byte(program + "ABCDEF"[:width] + suffix)
				stop := core.Parse(src, nil)
				end := core.RootEnd()
				core.ReleaseLast()
				if stop != "" || end != uint32(len(src)) {
					t.Errorf("trailing prefix %q: stop=%q end=%d, want %d", "ABCDEF"[:width]+suffix, stop, end, len(src))
				}
			}
		}
	})
}
