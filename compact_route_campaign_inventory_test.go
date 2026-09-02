package gotreesitter

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const compactRouteCampaignInventoryPath = "testdata/compact_route_campaign_inventory_v1.json"

type compactRouteCampaignInventory struct {
	Schema         string                              `json:"schema"`
	SourceRevision string                              `json:"source_revision"`
	Scope          string                              `json:"scope"`
	Entries        []compactRouteCampaignInventoryItem `json:"entries"`
}

type compactRouteCampaignInventoryItem struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Identifier        string `json:"identifier"`
	Owner             string `json:"owner"`
	Stage             int    `json:"stage"`
	Status            string `json:"status"`
	LifecycleControl  string `json:"lifecycle_control"`
	Gate              string `json:"gate"`
	FinalReceipt      string `json:"final_receipt"`
	DeletionCondition string `json:"deletion_condition"`
}

func TestCompactRouteCampaignInventory(t *testing.T) {
	data, err := os.ReadFile(compactRouteCampaignInventoryPath)
	if err != nil {
		t.Fatalf("open %s: %v", compactRouteCampaignInventoryPath, err)
	}
	if err := rejectCompactRouteLifecycleDuplicateKeys(data); err != nil {
		t.Fatalf("inventory contains invalid JSON structure: %v", err)
	}
	var inventory compactRouteCampaignInventory
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		t.Fatalf("decode %s: %v", compactRouteCampaignInventoryPath, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("inventory has trailing JSON data: %v", err)
	}
	if inventory.Schema != "gotreesitter/compact-route-campaign-inventory/v1" {
		t.Fatalf("schema = %q", inventory.Schema)
	}
	if !compactRouteLifecycleCommitPattern.MatchString(inventory.SourceRevision) {
		t.Fatalf("source_revision is not a commit hash: %q", inventory.SourceRevision)
	}
	if strings.TrimSpace(inventory.Scope) == "" || len(inventory.Entries) == 0 {
		t.Fatal("inventory requires a scope and at least one entry")
	}

	registry := loadCompactRouteLifecycleRegistry(t)
	controls := make(map[string]compactRouteLifecycleControl, len(registry.Controls))
	for _, control := range registry.Controls {
		controls[control.ID] = control
	}
	lifecycleEntries := make(map[string]compactRouteLifecycleEntry, len(registry.Entries))
	for _, entry := range registry.Entries {
		lifecycleEntries[entry.ID] = entry
	}
	allowedOwners := compactRouteLifecycleSet(t, "owners", registry.Vocabularies.Owners)
	allowedStatuses := compactRouteLifecycleSet(t, "statuses", registry.Vocabularies.Statuses)
	seenIDs := make(map[string]bool, len(inventory.Entries))
	seenIdentifiers := make(map[string]bool, len(inventory.Entries))
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inventory.Entries {
		if item.ID == "" || seenIDs[item.ID] {
			t.Errorf("inventory IDs must be unique and non-empty: %q", item.ID)
		}
		seenIDs[item.ID] = true
		if item.Identifier == "" || seenIdentifiers[item.Identifier] {
			t.Errorf("inventory identifiers must be unique and non-empty: %q", item.Identifier)
		}
		seenIdentifiers[item.Identifier] = true
		switch item.Kind {
		case "build_tag":
			if !compactRouteLifecycleBuildTagPattern.MatchString(item.Identifier) {
				t.Errorf("build tag %q has an invalid identifier", item.Identifier)
			}
		case "environment":
			if !compactRouteLifecycleEnvPattern.MatchString(item.Identifier) {
				t.Errorf("environment %q has an invalid identifier", item.Identifier)
			}
		default:
			t.Errorf("inventory item %q has unsupported kind %q", item.ID, item.Kind)
		}
		if item.Stage < 1 || item.Stage > 7 {
			t.Errorf("inventory item %q has unsupported stage %d", item.ID, item.Stage)
		}
		if !allowedOwners[item.Owner] || !allowedStatuses[item.Status] {
			t.Errorf("inventory item %q uses an unknown owner or status: %q/%q", item.ID, item.Owner, item.Status)
		}
		if strings.TrimSpace(item.Gate) == "" || strings.TrimSpace(item.DeletionCondition) == "" {
			t.Errorf("inventory item %q requires a gate and deletion condition", item.ID)
		}
		control, ok := controls[item.LifecycleControl]
		if !ok {
			t.Errorf("inventory item %q references unknown lifecycle control %q", item.ID, item.LifecycleControl)
		} else {
			switch item.Kind {
			case "build_tag":
				if !strings.Contains(control.Expression, item.Identifier) {
					t.Errorf("inventory item %q does not match control expression %q", item.ID, control.Expression)
				}
			case "environment":
				if control.Name != item.Identifier {
					t.Errorf("inventory item %q maps to control name %q", item.ID, control.Name)
				}
			}
		}
		finalEntry, ok := lifecycleEntries[item.FinalReceipt]
		if !ok {
			t.Errorf("inventory item %q references unknown final receipt %q", item.ID, item.FinalReceipt)
		} else if finalEntry.Stage != item.Stage {
			t.Errorf("inventory item %q stage %d does not match final receipt stage %d", item.ID, item.Stage, finalEntry.Stage)
		}
		if !compactRouteCampaignInventoryIdentifierPresent(t, root, item.Identifier) {
			t.Errorf("inventory identifier %q is not present in a tracked Go source", item.Identifier)
		}
	}
}

func compactRouteCampaignInventoryIdentifierPresent(t *testing.T, root, identifier string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".codex", ".claude", ".worktrees", "harness_out":
				return filepath.SkipDir
			}
			return nil
		}
		if found || filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(identifier)) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("search for inventory identifier %q: %v", identifier, err)
	}
	return found
}
