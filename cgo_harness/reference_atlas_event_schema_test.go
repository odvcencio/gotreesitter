package cgoharness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

type referenceAtlasSchema struct {
	Schema               string `json:"schema"`
	OrderedEventContract string `json:"ordered_event_contract"`
	Reference            struct {
		Commit string `json:"commit"`
	} `json:"reference"`
	Identity struct {
		RequiredFields           []string `json:"required_fields"`
		SemanticKeyFields        []string `json:"semantic_key_fields"`
		ExcludedPhysicalIdentity []string `json:"excluded_physical_identity"`
	} `json:"identity"`
	StatusDefinitions map[string]string     `json:"status_definitions"`
	Events            []referenceAtlasEvent `json:"events"`
	ClaimLimits       []string              `json:"claim_limits"`
}

type referenceAtlasEvent struct {
	ID             string               `json:"id"`
	Phase          string               `json:"phase"`
	RequiredFields []string             `json:"required_fields"`
	SemanticKey    []string             `json:"semantic_key"`
	C              referenceAtlasEngine `json:"c"`
	Go             referenceAtlasEngine `json:"go"`
}

type referenceAtlasEngine struct {
	Status string   `json:"status"`
	Fields []string `json:"fields"`
}

func TestReferenceAtlasEventSchemaIsCompleteAndHonest(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate the reference-atlas schema test")
	}
	path := filepath.Join(filepath.Dir(sourceFile), "work_count", "reference_atlas_event_schema_v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read reference-atlas schema: %v", err)
	}
	var schema referenceAtlasSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode reference-atlas schema: %v", err)
	}

	if schema.Schema != "gts-reference-atlas-event-schema/v1" {
		t.Fatalf("schema = %q", schema.Schema)
	}
	if schema.OrderedEventContract != "gts-reference-atlas-events/v1" {
		t.Fatalf("ordered event contract = %q", schema.OrderedEventContract)
	}
	if schema.Reference.Commit != "f5afe475deb7c0bae6407fb776c76824f717bb61" {
		t.Fatalf("reference commit = %q", schema.Reference.Commit)
	}
	if len(schema.Identity.RequiredFields) < 5 || len(schema.Identity.SemanticKeyFields) < 4 {
		t.Fatal("identity contract is too small to align semantic events")
	}
	if len(schema.Identity.ExcludedPhysicalIdentity) == 0 {
		t.Fatal("identity contract must exclude physical engine identifiers")
	}
	if len(schema.ClaimLimits) < 3 {
		t.Fatal("claim limits must state the evidence boundary")
	}

	wantStatuses := map[string]bool{"aggregate_only": true, "planned": true, "ordered": true}
	seen := make(map[string]bool, len(schema.Events))
	for index, event := range schema.Events {
		if event.ID == "" || event.Phase == "" {
			t.Fatalf("event %d has no id or phase", index)
		}
		if seen[event.ID] {
			t.Fatalf("event %q is duplicated", event.ID)
		}
		seen[event.ID] = true
		if len(event.RequiredFields) < 2 || len(event.SemanticKey) < 2 {
			t.Fatalf("event %q has an incomplete identity", event.ID)
		}
		validateReferenceAtlasEngine(t, event.ID, "C", event.C, wantStatuses)
		validateReferenceAtlasEngine(t, event.ID, "Go", event.Go, wantStatuses)
	}
	if len(schema.Events) != 24 {
		t.Fatalf("event count = %d, want 24", len(schema.Events))
	}
}

func validateReferenceAtlasEngine(t *testing.T, eventID, engine string, value referenceAtlasEngine, statuses map[string]bool) {
	t.Helper()
	if !statuses[value.Status] {
		t.Fatalf("event %q %s status %q is not an allowed evidence status", eventID, engine, value.Status)
	}
	if value.Status == "planned" && len(value.Fields) != 0 {
		t.Fatalf("event %q %s planned status lists fields", eventID, engine)
	}
	if value.Status == "aggregate_only" && len(value.Fields) == 0 {
		t.Fatalf("event %q %s aggregate status has no counter mapping", eventID, engine)
	}
	if value.Status == "ordered" {
		wantFields := []string{"event_seq", "source_start_byte", "source_end_byte", "parse_state", "symbol", "call_site", "event_kind", "outcome"}
		if !reflect.DeepEqual(value.Fields, wantFields) {
			t.Fatalf("event %q %s ordered fields=%v want=%v", eventID, engine, value.Fields, wantFields)
		}
	}
	for _, field := range value.Fields {
		if field == "" {
			t.Fatalf("event %q %s contains an empty field mapping", eventID, engine)
		}
		if filepath.Base(field) != field && field != "MergeEventCensusCounts.Attempts" && field != "MergeEventCensusCounts.Successes" && field != "MergeEventCensusCounts.LinkPayloadDeepAccepts" && field != "MergeEventCensusCounts.LinkPayloadShallowWouldAccept" && field != "MergeEventCensusCounts.LinkPayloadTests" && field != "MergeEventCensusCounts.LinkPayloadDeepRefusals" && field != "MergeEventCensusCounts.LinkPayloadPending" {
			// Keep qualified Go names explicit. Do not silently accept a path
			// or a physical identifier in the semantic contract.
			t.Fatalf("event %q %s field %q is not a semantic counter name", eventID, engine, field)
		}
	}
}

func Example_referenceAtlasEventSchema() {
	_, sourceFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(sourceFile), "work_count", "reference_atlas_event_schema_v1.json")
	data, _ := os.ReadFile(path)
	var schema referenceAtlasSchema
	_ = json.Unmarshal(data, &schema)
	fmt.Println(schema.Schema, len(schema.Events))
	// Output: gts-reference-atlas-event-schema/v1 24
}
