// Package grammars provides built-in and extension tree-sitter grammars with
// lazy loading. Most built-in grammars are currently shipped as compressed
// ts2go blobs, while extension grammars can come from grammargen-generated
// loaders. Use AllLanguages to enumerate available grammars, DetectLanguage to
// match by file extension or shebang, or call individual language functions
// (e.g. GoLanguage()) for direct access.
package grammars

import (
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/odvcencio/gotreesitter"
)

// GrammarSource describes where a LangEntry's language loader comes from.
type GrammarSource string

const (
	GrammarSourceUnknown GrammarSource = "unknown"
	// GrammarSourceTS2GoBlob marks an embedded .bin blob compiled by the
	// ts2go pipeline from upstream C parser tables.
	GrammarSourceTS2GoBlob GrammarSource = "ts2go_blob"
	// GrammarSourceGrammargen marks a language generated at runtime by
	// grammargen (extension grammars registered via RegisterExtension);
	// these have no embedded blob.
	GrammarSourceGrammargen GrammarSource = "grammargen"
	// GrammarSourceGrammargenBlob marks an embedded .bin blob compiled by
	// grammargen (cmd/grammargen -lr-split -bin ...), e.g. go.bin.
	GrammarSourceGrammargenBlob GrammarSource = "grammargen_blob"
)

// LangEntry holds a registered language with its grammar, extensions, and highlight query.
type LangEntry struct {
	Name               string
	Extensions         []string                      // e.g. [".go", ".mod"]
	Shebangs           []string                      // e.g. ["#!/usr/bin/env python"]
	Language           func() *gotreesitter.Language // lazy loader
	GrammarSource      GrammarSource                 // e.g. ts2go blob or grammargen
	HighlightQuery     string
	InheritHighlights  string                                                                 // language name to inherit highlight queries from (e.g. "javascript" for TypeScript)
	TagsQuery          string                                                                 // tree-sitter tags.scm query for symbol extraction
	TokenSourceFactory func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource // nil = use DFA
	Quality            ParseQuality                                                           // populated by AuditParseSupport

	// rawHighlightQuery preserves the entry's pre-composition highlight query
	// so re-running inheritance resolution (any Register call resets it) stays
	// idempotent instead of prepending parent queries a second time.
	rawHighlightQuery      string
	resolvedHighlightQuery string
	highlightRawCaptured   bool
}

var registry []LangEntry
var highlightInheritanceResolved bool

// registryMu guards registry, highlightInheritanceResolved, extIndex, and
// extensionAliases. Writers hold Lock(); readers hold RLock(). Never acquire
// registryMu while calling ensureBuiltinLanguagesRegistered — Register itself
// acquires the write lock, which would cause a deadlock.
var registryMu sync.RWMutex

// extIndex caches a suffix→LangEntry map for O(1) extension lookups in DetectLanguage.
// Invalidated (set to nil) whenever Register is called.
var extIndex map[string]*LangEntry

var (
	builtinRegistryOnce sync.Once
	builtinRegistryBusy atomic.Bool
)

func ensureBuiltinLanguagesRegistered() {
	builtinRegistryOnce.Do(func() {
		builtinRegistryBusy.Store(true)
		defer func() {
			builtinRegistryBusy.Store(false)
		}()
		registerBuiltinLanguages()
	})
}

// ensureReady guarantees that builtin languages are registered and the lazy
// metadata (highlight inheritance, extIndex) are up to date. Callers MUST NOT
// hold registryMu when calling this — ensureBuiltinLanguagesRegistered calls
// Register, which acquires the write lock.
func ensureReady() {
	ensureBuiltinLanguagesRegistered()

	registryMu.RLock()
	ready := highlightInheritanceResolved && extIndex != nil
	registryMu.RUnlock()

	if ready {
		return
	}

	// Slow path: acquire write lock and (re-)check before doing the work.
	registryMu.Lock()
	defer registryMu.Unlock()
	if !highlightInheritanceResolved {
		resolveHighlightInheritance()
	}
	if extIndex == nil {
		buildExtIndex()
	}
}

// Register adds a language to the registry. If an entry with the same name
// already exists, it is replaced so that grammar updates take effect.
func Register(entry LangEntry) {
	if !builtinRegistryBusy.Load() {
		ensureBuiltinLanguagesRegistered()
	}
	if !languageEnabled(entry.Name) {
		return
	}
	if entry.GrammarSource == "" {
		entry.GrammarSource = GrammarSourceUnknown
	}
	if entry.TokenSourceFactory == nil {
		entry.TokenSourceFactory = defaultTokenSourceFactory(entry.Name)
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	for i := range registry {
		if registry[i].Name == entry.Name {
			// Copies returned by AllLanguages and DetectLanguageByName retain the
			// private raw-query metadata. Preserve that metadata when callers
			// re-register an otherwise unchanged composed query, but recapture a
			// deliberately replaced public query as the new raw child query.
			if entry.highlightRawCaptured && entry.HighlightQuery == registry[i].resolvedHighlightQuery {
				entry.rawHighlightQuery = registry[i].rawHighlightQuery
			} else {
				entry.rawHighlightQuery = entry.HighlightQuery
				entry.highlightRawCaptured = true
			}
			entry.resolvedHighlightQuery = ""
			registry[i] = entry
			highlightInheritanceResolved = false
			extIndex = nil
			return
		}
	}
	entry.rawHighlightQuery = entry.HighlightQuery
	entry.highlightRawCaptured = true
	registry = append(registry, entry)
	highlightInheritanceResolved = false
	extIndex = nil
}

// RegisterExtension registers a grammar extension with the language registry.
// This enables file detection, fence highlighting, tags, and LSP support. The
// callback runs lazily on first access. Its result and error are cached.
// GrammarSource defaults to GrammarSourceGrammargen.
//
// Usage from an extension package:
//
//	func init() {
//	    grammars.RegisterExtension(grammars.ExtensionEntry{
//	        Name:           "danmuji",
//	        Extensions:     []string{".dmj"},
//	        Aliases:        []string{"dmj"},
//	        GenerateLanguage: func() (*gotreesitter.Language, error) {
//	            return grammargen.GenerateLanguage(danmuji.Grammar())
//	        },
//	        HighlightQuery: danmuji.HighlightQueries(),
//	    })
//	}
type ExtensionEntry struct {
	Name              string
	Extensions        []string // file extensions: [".dmj", ".dingo", ".fw"]
	Aliases           []string // markdown fence aliases: ["dmj", "danmuji"]
	GenerateLanguage  func() (*gotreesitter.Language, error)
	GrammarSource     GrammarSource // empty defaults to GrammarSourceGrammargen
	HighlightQuery    string
	InheritHighlights string // parent language for highlight query composition (e.g. "go")
	TagsQuery         string // tree-sitter tags query for symbol extraction
}

// RegisterExtension registers a grammar extension for file detection and
// markdown code fence highlighting.
func RegisterExtension(ext ExtensionEntry) {
	grammarSource := ext.GrammarSource
	if grammarSource == "" {
		grammarSource = GrammarSourceGrammargen
	}
	var (
		generateOnce sync.Once
		cached       *gotreesitter.Language
		cachedErr    error
	)
	loader := func() *gotreesitter.Language {
		generateOnce.Do(func() {
			cached, cachedErr = ext.GenerateLanguage()
		})
		if cachedErr != nil {
			return nil
		}
		return cached
	}

	Register(LangEntry{
		Name:              ext.Name,
		Extensions:        ext.Extensions,
		Language:          loader,
		GrammarSource:     grammarSource,
		HighlightQuery:    ext.HighlightQuery,
		InheritHighlights: ext.InheritHighlights,
		TagsQuery:         ext.TagsQuery,
	})

	// Register aliases for markdown fence resolution under the write lock so
	// readers in DetectLanguageByName see a consistent extensionAliases state.
	if len(ext.Aliases) > 0 {
		registryMu.Lock()
		for _, alias := range ext.Aliases {
			if alias != ext.Name {
				extensionAliases[alias] = ext.Name
			}
		}
		registryMu.Unlock()
	}
}

// extensionAliases maps markdown fence aliases to canonical names.
// Protected by registryMu.
var extensionAliases = map[string]string{}

// defaultHighlightInherits records the upstream tree-sitter highlight
// inheritance convention for grammars whose generated registry entries do
// not populate InheritHighlights: their bundled highlights.scm contains only
// language-specific additions and expects the parent's query ahead of it.
// An entry's explicit InheritHighlights always takes priority.
var defaultHighlightInherits = map[string]string{
	"blade":      "html",
	"cpp":        "c",
	"cuda":       "cpp",
	"glsl":       "c",
	"objc":       "c",
	"svelte":     "html",
	"templ":      "go",
	"typescript": "javascript",
	"tsx":        "typescript",
}

// blockedDefaultHighlightInherits keeps upstream inheritance metadata visible
// when the locked child grammar cannot compile the current parent's query.
// The edge remains fail-closed until the grammar/query revisions are aligned.
var blockedDefaultHighlightInherits = map[string]string{
	"cuda": "the locked CUDA grammar lacks C++'s module_name node",
}

// resolveHighlightInheritance composes highlight queries for languages that
// inherit from a parent, walking multi-level chains (tsx -> typescript ->
// javascript) regardless of registration order and guarding against cycles.
// The parent is the entry's InheritHighlights, falling back to the upstream
// convention in defaultHighlightInherits. MUST be called with registryMu
// .Lock() held. Callers are responsible for ensuring builtins are registered
// first.
func resolveHighlightInheritance() {
	if highlightInheritanceResolved {
		return
	}
	highlightInheritanceResolved = true

	index := make(map[string]int, len(registry))
	for i := range registry {
		index[registry[i].Name] = i
		if !registry[i].highlightRawCaptured {
			registry[i].rawHighlightQuery = registry[i].HighlightQuery
			registry[i].highlightRawCaptured = true
		}
	}
	const (
		resolveVisiting uint8 = iota + 1
		resolveDone
	)
	state := make(map[string]uint8, len(registry))
	composed := make(map[string]string, len(registry))
	cyclic := make(map[string]bool, len(registry))

	var resolve func(name string) (string, bool)
	resolve = func(name string) (string, bool) {
		switch state[name] {
		case resolveVisiting:
			return "", true
		case resolveDone:
			return composed[name], cyclic[name]
		}
		i, ok := index[name]
		if !ok {
			return "", false
		}
		state[name] = resolveVisiting
		query := registry[i].rawHighlightQuery
		parent := registry[i].InheritHighlights
		blocked := false
		if parent == "" {
			parent = defaultHighlightInherits[name]
			_, blocked = blockedDefaultHighlightInherits[name]
		}
		if parent != "" && !blocked {
			parentQuery, parentCyclic := resolve(parent)
			if parentCyclic {
				// Fail closed for the entire invalid inheritance chain. Every
				// affected language keeps exactly its own raw query.
				cyclic[name] = true
			} else if strings.TrimSpace(parentQuery) != "" {
				if strings.TrimSpace(query) == "" {
					// Prepend parent query so child overrides win (last match wins in tree-sitter).
					query = parentQuery
				} else {
					query = parentQuery + "\n" + query
				}
			}
		}
		composed[name] = query
		state[name] = resolveDone
		return query, cyclic[name]
	}

	for i := range registry {
		registry[i].HighlightQuery, _ = resolve(registry[i].Name)
		registry[i].resolvedHighlightQuery = registry[i].HighlightQuery
	}
}

// buildExtIndex builds a suffix→LangEntry map from the current registry.
// MUST be called with registryMu.Lock() held.
func buildExtIndex() {
	idx := make(map[string]*LangEntry, len(registry)*2)
	for i := range registry {
		for _, ext := range registry[i].Extensions {
			// First registration wins; mirrors the O(n²) loop behaviour.
			if _, exists := idx[ext]; !exists {
				idx[ext] = &registry[i]
			}
		}
	}
	extIndex = idx
}

// DetectLanguage returns the LangEntry for a filename, or nil if unknown.
// Checks in order: exact filename match (linguist), registry extensions,
// then linguist extended extensions. Exact filenames take priority over
// suffix matching so that e.g. ".tmux.conf" resolves to bash rather than
// matching the generic ".conf" extension.
func DetectLanguage(filename string) *LangEntry {
	ensureReady()
	registryMu.RLock()
	defer registryMu.RUnlock()

	// 1. Exact filename match (e.g., "Makefile", "Dockerfile", ".bashrc",
	//    "nginx.conf"). Most specific, so checked first.
	base := path.Base(filename)
	if grammarName, ok := linguistFilenames[base]; ok {
		return lookupByName(grammarName)
	}

	// 2. Match by registry extensions — O(1) map lookup.
	// Collect up to 4 dot-suffixes from longest to shortest using a stack
	// array to avoid heap allocation (e.g. ".blade.php" before ".php").
	var suffixes [4]string
	nsuf := 0
	for i := len(base) - 1; i > 0 && nsuf < len(suffixes); i-- {
		if base[i] == '.' {
			suffixes[nsuf] = base[i:]
			nsuf++
		}
	}
	// suffixes[0] is shortest; check longest (highest index) first.
	for i := nsuf - 1; i >= 0; i-- {
		if entry, ok := extIndex[suffixes[i]]; ok {
			return entry
		}
	}

	// 3. Linguist extended extensions (e.g., ".mk" for make, ".rake" for ruby).
	ext := strings.ToLower(path.Ext(filename))
	if ext != "" {
		if grammarName, ok := linguistExtensions[ext]; ok {
			return lookupByName(grammarName)
		}
	}

	return nil
}

// DetectLanguageByShebang checks the first line of content for shebang matches.
// Handles both "#!/usr/bin/env python3" and "#!/usr/bin/python3" forms.
func DetectLanguageByShebang(firstLine string) *LangEntry {
	ensureReady()
	registryMu.RLock()
	defer registryMu.RUnlock()

	// 1. Registry shebangs (exact prefix match).
	for i := range registry {
		for _, shebang := range registry[i].Shebangs {
			if strings.HasPrefix(firstLine, shebang) {
				return &registry[i]
			}
		}
	}

	// 2. Extract interpreter from shebang and look up in linguist map.
	interp := extractInterpreter(firstLine)
	if interp != "" {
		if grammarName, ok := linguistInterpreters[interp]; ok {
			return lookupByName(grammarName)
		}
	}

	return nil
}

// extractInterpreter parses a shebang line and returns the interpreter name.
// Handles "#!/usr/bin/env python3" → "python3" and "#!/usr/bin/python3" → "python3".
func extractInterpreter(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") {
		return ""
	}
	line = line[2:]
	line = strings.TrimSpace(line)

	// Split into path and args.
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}

	// Get the binary name from the path.
	binary := path.Base(parts[0])

	// If it's "env", the interpreter is the next non-flag, non-VAR=val argument.
	if binary == "env" {
		for _, arg := range parts[1:] {
			if strings.HasPrefix(arg, "-") {
				continue // skip flags like -S, -u
			}
			if strings.Contains(arg, "=") {
				continue // skip VAR=value env assignments
			}
			return strings.ToLower(arg)
		}
		return ""
	}

	return strings.ToLower(binary)
}

// AllLanguages returns all registered languages. This is a metadata-only
// operation — it does NOT load grammar parse tables or decompress blobs.
// Languages that lack an explicit TagsQuery will have an empty TagsQuery
// field; call [ResolveTagsQuery] when you actually need the inferred query.
func AllLanguages() []LangEntry {
	ensureReady()
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]LangEntry, len(registry))
	copy(out, registry)
	return out
}

// ResolveTagsQuery returns the tags query for a LangEntry, computing an
// inferred query from grammar symbols on first call if the entry lacks an
// explicit TagsQuery. The inferred result is cached by language name.
// This may trigger grammar loading for languages without an explicit TagsQuery.
func ResolveTagsQuery(entry LangEntry) string {
	if q := strings.TrimSpace(entry.TagsQuery); q != "" {
		return entry.TagsQuery
	}
	return inferredTagsQuery(entry)
}

// lookupByName returns the LangEntry with the given grammar name, or nil.
// MUST be called with registryMu.RLock() or Lock() held.
func lookupByName(name string) *LangEntry {
	for i := range registry {
		if registry[i].Name == name {
			return &registry[i]
		}
	}
	return nil
}

// normalizeLinguistKey lowercases and trims input, preserving special
// characters (+, #, etc.) so "C++" and "F#" map correctly.
func normalizeLinguistKey(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

// DetectLanguageByName returns the LangEntry for any linguist canonical name,
// alias, or gotreesitter grammar name. Returns nil if unknown.
//
// Accepts: "C++", "cpp", "Go", "golang", "Shell", "bash", "F#", "fsharp", etc.
// Direct grammar names always take priority over linguist aliases to prevent
// shadowing (e.g., "eex" resolves to the eex grammar, not heex via alias).
func DetectLanguageByName(name string) *LangEntry {
	ensureReady()
	key := normalizeLinguistKey(name)

	registryMu.RLock()
	defer registryMu.RUnlock()

	// Direct grammar name takes priority over alias mapping.
	if entry := lookupByName(key); entry != nil {
		return entry
	}
	if grammarName, ok := linguistToGrammar[key]; ok {
		return lookupByName(grammarName)
	}
	// Check extension aliases (e.g., "dmj" → "danmuji", "fw" → "ferrous-wheel")
	if canonical, ok := extensionAliases[key]; ok {
		return lookupByName(canonical)
	}
	return nil
}

// DisplayName returns the linguist canonical display name for a language
// (e.g., "C++" for cpp, "JavaScript" for javascript). Falls back to
// title-casing the grammar name if no linguist match exists.
func DisplayName(entry *LangEntry) string {
	if entry == nil {
		return ""
	}
	if dn, ok := grammarDisplayNames[entry.Name]; ok {
		return dn
	}
	// Fallback: title-case with underscores as spaces.
	words := strings.Split(entry.Name, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
