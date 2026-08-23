package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type retiredDispatchRouteReceipt struct {
	name               string
	tree               *gotreesitter.Tree
	incrementalProfile gotreesitter.IncrementalParseProfile
}

func retiredDispatchRouteReceipts(
	t *testing.T,
	lang *gotreesitter.Language,
	baseSource []byte,
) []retiredDispatchRouteReceipt {
	return retiredDispatchRouteReceiptsWithCompactPolicy(t, lang, baseSource, true)
}

func retiredDispatchRouteReceiptsAllowCompactFallback(
	t *testing.T,
	lang *gotreesitter.Language,
	baseSource []byte,
) []retiredDispatchRouteReceipt {
	return retiredDispatchRouteReceiptsWithCompactPolicy(t, lang, baseSource, false)
}

func retiredDispatchRouteReceiptsAllowCompactFallbackExactSource(
	t *testing.T,
	lang *gotreesitter.Language,
	source []byte,
) []retiredDispatchRouteReceipt {
	return retiredDispatchRouteReceiptsWithCompactPolicyAndRoutes(
		t,
		lang,
		source,
		false,
		false,
		false,
	)
}

func retiredDispatchRouteReceiptsAllowCompactAndForestFallbackExactSource(
	t *testing.T,
	lang *gotreesitter.Language,
	source []byte,
) []retiredDispatchRouteReceipt {
	return retiredDispatchRouteReceiptsWithCompactPolicyAndRoutes(
		t,
		lang,
		source,
		false,
		false,
		true,
	)
}

func retiredDispatchRouteReceiptsWithCompactPolicy(
	t *testing.T,
	lang *gotreesitter.Language,
	baseSource []byte,
	requireDirectCompact bool,
) []retiredDispatchRouteReceipt {
	return retiredDispatchRouteReceiptsWithCompactPolicyAndRoutes(
		t,
		lang,
		baseSource,
		requireDirectCompact,
		true,
		false,
	)
}

func retiredDispatchRouteReceiptsWithCompactPolicyAndRoutes(
	t *testing.T,
	lang *gotreesitter.Language,
	baseSource []byte,
	requireDirectCompact bool,
	appendTrailingNewline bool,
	allowForestFallback bool,
) []retiredDispatchRouteReceipt {
	t.Helper()

	source := append(append([]byte(nil), baseSource...), '\n')
	if !appendTrailingNewline {
		source = append([]byte(nil), baseSource...)
	}

	productionParser := gotreesitter.NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse failed: %v", err)
	}
	t.Cleanup(production.Release)
	if production.ParseRuntime().ForestFastPath {
		t.Fatal("production parse reported the forest route")
	}

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(lang)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatalf("compact parse failed: %v", err)
	}
	t.Cleanup(compact.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	direct := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
	fallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
	if requireDirectCompact && !direct || !requireDirectCompact && !direct && !fallback {
		t.Fatalf(
			"compact route counters routed=%d/%d fallback=%d/%d: %s",
			routedBefore,
			routedAfter,
			fallbackBefore,
			fallbackAfter,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	if direct {
		t.Logf("compact route=direct")
	} else {
		t.Logf("compact route=fallback reason=%q", gotreesitter.AdmissionCandidateLastFallbackReason())
	}

	forestParser := gotreesitter.NewParser(lang)
	forest, ok := forestParser.ParseForestExperimental(source)
	forestName := "forest"
	if !ok || forest == nil {
		offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
		if !allowForestFallback {
			t.Fatalf("forest route declined at %d symbol=%d reason=%s", offset, symbol, reason)
		}
		t.Logf("forest route=fallback offset=%d symbol=%d reason=%q", offset, symbol, reason)
		var fallbackErr error
		forest, fallbackErr = forestParser.Parse(source)
		if fallbackErr != nil {
			t.Fatalf("forest fallback parse failed: %v", fallbackErr)
		}
		forestName = "forest-fallback"
	}
	t.Cleanup(forest.Release)
	if forestName == "forest" && !forest.ParseRuntime().ForestFastPath {
		t.Fatal("forest parse did not report the forest route")
	}

	oldParser := gotreesitter.NewParser(lang)
	oldParser.SetAdmissionCandidateRoute(false)
	oldTree, err := oldParser.Parse(baseSource)
	if err != nil {
		t.Fatalf("incremental base parse failed: %v", err)
	}
	t.Cleanup(oldTree.Release)
	endPoint := retiredDispatchPointAtByte(baseSource, len(baseSource))
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(len(baseSource)),
		OldEndByte:  uint32(len(baseSource)),
		NewEndByte:  uint32(len(baseSource)),
		StartPoint:  endPoint,
		OldEndPoint: endPoint,
		NewEndPoint: endPoint,
	}
	if appendTrailingNewline {
		edit.NewEndByte = uint32(len(source))
		edit.NewEndPoint = gotreesitter.Point{Row: endPoint.Row + 1}
	}
	oldTree.Edit(edit)
	incremental, incrementalProfile, err := oldParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatalf("incremental parse failed: %v", err)
	}
	t.Cleanup(incremental.Release)

	receipts := []retiredDispatchRouteReceipt{
		{name: "production", tree: production},
		{name: "compact", tree: compact},
		{name: forestName, tree: forest},
		{name: "incremental", tree: incremental, incrementalProfile: incrementalProfile},
	}
	want, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range receipts[1:] {
		got, inspectErr := benchfixtures.InspectGoTree(receipt.tree.RootNode(), lang)
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		if got.SHA256 != want.SHA256 {
			t.Fatalf("%s digest = %s, want production %s", receipt.name, got.SHA256, want.SHA256)
		}
	}
	return receipts
}

func retiredDispatchPointAtByte(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for index, value := range source {
		if index == offset {
			break
		}
		if value == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}
