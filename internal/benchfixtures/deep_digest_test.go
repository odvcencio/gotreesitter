package benchfixtures

import "testing"

func TestDeepTreeDigestContract(t *testing.T) {
	nodes := []DeepNode{
		{
			Type:       "source_file",
			EndByte:    7,
			EndColumn:  7,
			Named:      true,
			ChildCount: 1,
		},
		{
			Type:        "identifier",
			Field:       "name",
			StartByte:   1,
			EndByte:     7,
			StartColumn: 1,
			EndColumn:   7,
			Named:       true,
		},
	}
	digest := NewDeepTreeDigest()
	for _, node := range nodes {
		if err := digest.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	got, err := digest.Hex()
	if err != nil {
		t.Fatal(err)
	}
	const want = "39beda3e780c158c7b54241561221ab56c573944b272b934ded1f286ede3ebb0"
	if got != want {
		t.Fatalf("digest=%s want=%s", got, want)
	}
}

func TestDeepTreeDigestFailsClosed(t *testing.T) {
	incomplete := NewDeepTreeDigest()
	if err := incomplete.Add(DeepNode{Type: "root", ChildCount: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := incomplete.Hex(); err == nil {
		t.Fatal("incomplete tree unexpectedly produced a digest")
	}

	extra := NewDeepTreeDigest()
	if err := extra.Add(DeepNode{Type: "root"}); err != nil {
		t.Fatal(err)
	}
	if err := extra.Add(DeepNode{Type: "extra"}); err == nil {
		t.Fatal("extra node unexpectedly admitted")
	}
}

func TestDeepTreeInspectionRecordsNodeKindsDefensively(t *testing.T) {
	digest := NewDeepTreeDigest()
	for _, node := range []DeepNode{
		{Type: "root", ChildCount: 2},
		{Type: "child"},
		{Type: "child"},
	} {
		if err := digest.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	inspection, err := digest.Inspection()
	if err != nil {
		t.Fatal(err)
	}
	if inspection.NodeKinds["root"] != 1 || inspection.NodeKinds["child"] != 2 {
		t.Fatalf("node kinds=%v", inspection.NodeKinds)
	}
	inspection.NodeKinds["child"] = 99
	again, err := digest.Inspection()
	if err != nil {
		t.Fatal(err)
	}
	if again.NodeKinds["child"] != 2 {
		t.Fatalf("inspection leaked mutable coverage: %v", again.NodeKinds)
	}
}

// The NUL token digests as "" in both spellings that blobs use for its name.
func TestDeepDigestTypeNameReadsBothNULSpellings(t *testing.T) {
	for name, want := range map[string]string{"\\0": "", "\x00": "", "": "", "0": "0", "\\": "\\", "end": "end"} {
		if got := deepDigestTypeName(name); got != want {
			t.Errorf("deepDigestTypeName(%q) = %q, want %q", name, got, want)
		}
	}
}
