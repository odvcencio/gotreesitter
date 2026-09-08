package grammars

import "strings"

// GrammarMaintenanceClass describes gotreesitter's source custody for a
// built-in grammar.
type GrammarMaintenanceClass string

const (
	GrammarMaintenanceMirror GrammarMaintenanceClass = "mirror"
	GrammarMaintenanceLead   GrammarMaintenanceClass = "lead"
	GrammarMaintenanceOwn    GrammarMaintenanceClass = "own"
)

// GrammarOwnership records the priority tier and source custody for a grammar.
type GrammarOwnership struct {
	Tier             uint8
	MaintenanceClass GrammarMaintenanceClass
	UpstreamRepo     string
	UpstreamCommit   string
	UpstreamLicense  string
}

var grammarOwnershipManifest = map[string]GrammarOwnership{
	"bash":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"c":          {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"cpp":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"c_sharp":    {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"css":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"elixir":     {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"go":         {Tier: 1, MaintenanceClass: GrammarMaintenanceOwn, UpstreamRepo: "https://github.com/tree-sitter/tree-sitter-go", UpstreamCommit: "2346a3ab1bb3857b48b29d779a1ef9799a248cd7", UpstreamLicense: "MIT"},
	"graphql":    {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"hcl":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"html":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"java":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"javascript": {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"json":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"kotlin":     {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"lua":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"markdown":   {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"nix":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"php":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"python":     {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"ruby":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"rust":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"scala":      {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"sql":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"swift":      {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"toml":       {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"tsx":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"typescript": {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"xml":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
	"yaml":       {Tier: 1, MaintenanceClass: GrammarMaintenanceOwn, UpstreamRepo: "https://github.com/tree-sitter-grammars/tree-sitter-yaml", UpstreamCommit: "4463985dfccc640f3d6991e3396a2047610cf5f8", UpstreamLicense: "MIT"},
	"zig":        {Tier: 1, MaintenanceClass: GrammarMaintenanceMirror},
}

// GrammarOwnershipFor returns the declared ownership of a built-in grammar.
func GrammarOwnershipFor(name string) (GrammarOwnership, bool) {
	ownership, ok := grammarOwnershipManifest[strings.ToLower(strings.TrimSpace(name))]
	return ownership, ok
}
