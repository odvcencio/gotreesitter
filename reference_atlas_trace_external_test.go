//go:build gts_workcount

package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestReferenceAtlasTraceEmitsOrderedGoEventsFromProduction(t *testing.T) {
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	parser.SetAdmissionCandidateRoute(false)
	gotreesitter.BeginDiagnosticWorkCount()
	gotreesitter.BeginDiagnosticSemanticPhaseTrace()
	gotreesitter.BeginDiagnosticReferenceAtlasTrace()
	tree, err := parser.Parse([]byte("package p\nvar x = 1\n"))
	atlas := gotreesitter.EndDiagnosticReferenceAtlasTrace()
	semantic := gotreesitter.EndDiagnosticSemanticPhaseTrace()
	_ = gotreesitter.EndDiagnosticWorkCount()
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("parse returned no tree")
	}
	tree.Release()

	if semantic.EventsSeen == 0 || atlas.EventsSeen == 0 || atlas.EventsDropped != 0 {
		t.Fatalf("semantic events=%d atlas events=%d dropped=%d", semantic.EventsSeen, atlas.EventsSeen, atlas.EventsDropped)
	}
	wantKinds := map[string]bool{
		"action_table.lookup": true,
		"parser.shift":        true,
		"parser.reduce":       true,
		"parser.accept":       true,
	}
	seenKinds := make(map[string]bool)
	for index, event := range atlas.Events {
		if event.EventSeq != uint64(index+1) || event.CallSite == "" || event.EventKind == "" {
			t.Fatalf("invalid ordered event %d: %+v", index, event)
		}
		if event.SourceEndByte < event.SourceStartByte {
			t.Fatalf("event %d has reversed span: %+v", index, event)
		}
		if !wantKinds[event.EventKind] {
			t.Fatalf("unexpected event kind %q", event.EventKind)
		}
		seenKinds[event.EventKind] = true
	}
	for kind := range wantKinds {
		if !seenKinds[kind] {
			t.Fatalf("ordered Go trace did not emit %q", kind)
		}
	}
}
