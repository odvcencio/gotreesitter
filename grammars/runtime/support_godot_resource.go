//go:build !grammar_subset || grammar_subset_godot_resource

package grammarruntime

// RegisterGodot_resourceSupport registers the scanner support for godot_resource.
func RegisterGodot_resourceSupport() {
	RegisterExternalScanner("godot_resource", GodotResourceExternalScanner{})
}
