//go:build !grammar_subset || grammar_subset_angular || grammar_subset_astro || grammar_subset_blade || grammar_subset_html || grammar_subset_svelte || grammar_subset_vue

package grammarruntime

import (
	"bytes"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestHTMLDeserializeTagsIntoReusesBackingArray(t *testing.T) {
	tags := []htmlTag{
		{tagType: htmlTagDiv},
		{tagType: htmlTagCustom, customName: "CUSTOM-ELEMENT"},
	}
	buf := make([]byte, 64)
	n := htmlSerializeTags(tags, buf)
	if n == 0 {
		t.Fatal("htmlSerializeTags returned empty snapshot")
	}

	dst := make([]htmlTag, 1, 4)
	backing := &dst[:cap(dst)][0]
	got := htmlDeserializeTagsInto(dst, buf[:n])
	if len(got) != len(tags) {
		t.Fatalf("htmlDeserializeTagsInto len = %d, want %d", len(got), len(tags))
	}
	if &got[:cap(got)][0] != backing {
		t.Fatal("htmlDeserializeTagsInto did not reuse destination backing array")
	}
	for i := range tags {
		if !htmlTagEq(&got[i], &tags[i]) {
			t.Fatalf("tag %d = %#v, want %#v", i, got[i], tags[i])
		}
	}
}

func TestHTMLTagCheckpointIsExactAndFailClosed(t *testing.T) {
	t.Run("empty_state_is_present", func(t *testing.T) {
		buf := make([]byte, 4)
		if n := htmlSerializeTags(nil, buf); n != 4 {
			t.Fatalf("empty checkpoint size = %d, want 4", n)
		}
		if got := htmlDeserializeTagsInto([]htmlTag{{tagType: htmlTagDiv}}, buf); len(got) != 0 || got == nil {
			t.Fatalf("empty checkpoint decoded as %#v, want non-nil empty state", got)
		}
	})

	tags := []htmlTag{
		{tagType: htmlTagHtml},
		{tagType: htmlTagCustom, customName: "CUSTOM-ELEMENT"},
		{tagType: htmlTagInterpolation},
	}
	required := 4 + 1 + 2 + len(tags[1].customName) + 1
	t.Run("exact_capacity", func(t *testing.T) {
		buf := make([]byte, required)
		if n := htmlSerializeTags(tags, buf); n != required {
			t.Fatalf("checkpoint size = %d, want %d", n, required)
		}
		got := htmlDeserializeTagsInto(nil, buf)
		if len(got) != len(tags) {
			t.Fatalf("decoded tag count = %d, want %d", len(got), len(tags))
		}
		for i := range tags {
			if !htmlTagEq(&got[i], &tags[i]) {
				t.Fatalf("decoded tag %d = %#v, want %#v", i, got[i], tags[i])
			}
		}
	})

	t.Run("insufficient_capacity", func(t *testing.T) {
		buf := bytes.Repeat([]byte{0xa5}, required-1)
		before := append([]byte(nil), buf...)
		if n := htmlSerializeTags(tags, buf); n != 0 {
			t.Fatalf("short buffer serialized %d bytes, want absent checkpoint", n)
		}
		if !bytes.Equal(buf, before) {
			t.Fatal("failed serialization partially modified the destination")
		}
	})

	t.Run("custom_name_too_long", func(t *testing.T) {
		prefix := strings.Repeat("X", 255)
		left := []htmlTag{{tagType: htmlTagCustom, customName: prefix + "A"}}
		right := []htmlTag{{tagType: htmlTagCustom, customName: prefix + "B"}}
		buf := make([]byte, 4096)
		if n := htmlSerializeTags(left, buf); n != 0 {
			t.Fatalf("left colliding state serialized %d bytes, want absent", n)
		}
		if n := htmlSerializeTags(right, buf); n != 0 {
			t.Fatalf("right colliding state serialized %d bytes, want absent", n)
		}
	})

	t.Run("tag_depth_too_large", func(t *testing.T) {
		deep := make([]htmlTag, 1<<16)
		if n := htmlSerializeTags(deep, make([]byte, len(deep)+4)); n != 0 {
			t.Fatalf("unrepresentable depth serialized %d bytes, want absent", n)
		}
	})
}

func TestHTMLTagCheckpointRejectsMalformedEncodings(t *testing.T) {
	valid := make([]byte, 16)
	n := htmlSerializeTags([]htmlTag{{tagType: htmlTagDiv}}, valid)
	valid = valid[:n]

	cases := map[string][]byte{
		"short_header":            {0, 0, 0},
		"partial_count":           {0, 0, 1, 0},
		"truncated_tag":           {1, 0, 1, 0},
		"truncated_custom_length": {1, 0, 1, 0, byte(htmlTagCustom)},
		"truncated_custom_name":   {1, 0, 1, 0, byte(htmlTagCustom), 2, 'X'},
		"invalid_tag_type":        {1, 0, 1, 0, byte(htmlTagInterpolation + 1)},
		"trailing_bytes":          append(append([]byte(nil), valid...), 0),
	}
	for name, snapshot := range cases {
		t.Run(name, func(t *testing.T) {
			dst := []htmlTag{{tagType: htmlTagDiv}}
			if got := htmlDeserializeTagsInto(dst, snapshot); got != nil {
				t.Fatalf("malformed checkpoint decoded as %#v, want nil state", got)
			}
		})
	}
}

func TestHTMLScannerIncrementalCapabilities(t *testing.T) {
	var scanner gts.ExternalScanner = HTMLExternalScanner{}
	reuse, ok := scanner.(gts.IncrementalReuseExternalScanner)
	if !ok || !reuse.SupportsIncrementalReuse() {
		t.Fatal("HTML scanner is not certified for incremental reuse")
	}
	errorTreeReuse, ok := scanner.(gts.ErrorTreeIncrementalReuseExternalScanner)
	if !ok || errorTreeReuse.SupportsIncrementalReuseFromErrorTree() {
		t.Fatal("HTML scanner did not retain its error-tree fail-closed boundary")
	}
	checkpointed, ok := scanner.(gts.CheckpointedExternalScanner)
	if !ok || !checkpointed.UsesExternalScannerCheckpoints() {
		t.Fatal("HTML scanner does not advertise checkpointed state")
	}
	failurePreserving, ok := scanner.(gts.FailurePreservingExternalScanner)
	if !ok || !failurePreserving.PreservesStateOnScanFailure() {
		t.Fatal("HTML scanner does not advertise failed-scan state preservation")
	}
}
