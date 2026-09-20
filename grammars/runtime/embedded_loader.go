package grammarruntime

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

// externalScannerRegistry maps language names (e.g. "javascript") to their
// hand-written external scanners. Populated by zzz_scanner_attachments.go
// init() before any Language function is called.
var externalScannerRegistry = map[string]gotreesitter.ExternalScanner{}

// externalLexStatesRegistry maps language names to their external lex states
// tables, matching C tree-sitter's ts_external_scanner_states. Populated by
// zzz_scanner_attachments.go init() for grammars that need precise external
// token validity filtering.
var externalLexStatesRegistry = map[string][][]bool{}

// RegisterExternalScanner registers an external scanner for a language name.
// This is called during init() by zzz_scanner_attachments.go.
func RegisterExternalScanner(name string, s gotreesitter.ExternalScanner) {
	externalScannerRegistry[name] = s
}

// RegisterExternalLexStates registers the external lex states table for a
// language, matching C tree-sitter's ts_external_scanner_states.
func RegisterExternalLexStates(name string, states [][]bool) {
	externalLexStatesRegistry[name] = states
}

// ReservedWordTable holds one language's ABI 15 reserved-word data
// (ts_reserved_words) plus the provenance AttachLanguageSupport needs to
// confirm the table still matches the embedded language's decoded symbol
// table before it writes anything onto that language.
type ReservedWordTable struct {
	// MaxSetSize is the reserved-word set stride: C's ts_reserved_words row
	// width, matching gotreesitter.Language.MaxReservedWordSetSize.
	MaxSetSize int
	// Words is the flat reserved-word array, stride MaxSetSize, matching
	// gotreesitter.Language.ReservedWords layout.
	Words []gotreesitter.Symbol
	// SymbolCount is the total ts_symbol_names entry count of the source
	// parser.c this table was generated against.
	SymbolCount int
	// SymbolNames records, for every symbol ID this table references, that
	// symbol's C ts_symbol_names display name at generation time.
	SymbolNames map[gotreesitter.Symbol]string
}

// reservedWordsRegistry maps language names to their ABI 15 reserved-word
// table. Populated by generated runtime/*_reserved_words_gen.go sidecars
// (cmd/ts2go -reservedwords-only).
var reservedWordsRegistry = map[string]ReservedWordTable{}

// RegisterReservedWords registers the ABI 15 reserved-word table for a
// language name. This is called during init() by generated
// runtime/*_reserved_words_gen.go sidecars.
func RegisterReservedWords(name string, table ReservedWordTable) {
	reservedWordsRegistry[name] = table
}

// LookupReservedWords returns the registered ABI 15 reserved-word table for
// the given language name, and whether one is registered.
func LookupReservedWords(name string) (ReservedWordTable, bool) {
	table, ok := reservedWordsRegistry[name]
	return table, ok
}

// attachRegisteredReservedWords attaches the registered ABI 15 reserved-word
// table for name onto lang, when doing so is provably safe.
//
// The blobs for a handful of legacy languages predate cmd/ts2go's
// reserved-word extraction, so their decoded Language carries no
// ReservedWords data even though their lex modes reference reserved-word set
// IDs. This sidecar mechanism supplies that missing data after the fact,
// generated separately from the pinned blob. A language drops out of this
// group once its blob is regenerated with a ts2go build that extracts
// ts_reserved_words directly (see ocaml, retired 2026-09-20): the freshly
// decoded Language then already carries ReservedWords, so the fail-closed
// check below skips the sidecar and its file becomes dead weight.
//
// Because the sidecar and the blob can drift independently, the attach fails
// closed: it requires every one of the following before it writes anything
// onto lang.
//
//   - lang carries no reserved-word data yet. A blob that already encodes
//     ts_reserved_words itself always wins over this sidecar.
//   - lang's decoded symbol count matches the table's recorded symbol count
//     exactly, so no symbol ID has shifted since generation.
//   - lang.SymbolNames[id] equals the table's recorded name for every symbol
//     ID the table references.
//   - every lex mode's ReservedWordSetID indexes a row inside the table.
//
// Any mismatch skips the attach silently. lang then keeps parsing with no
// reserved-word promotion, exactly as it did before this sidecar existed.
func attachRegisteredReservedWords(name string, lang *gotreesitter.Language) bool {
	if lang == nil {
		return false
	}
	table, ok := reservedWordsRegistry[name]
	if !ok {
		return false
	}
	if len(lang.ReservedWords) != 0 || lang.MaxReservedWordSetSize != 0 {
		return false
	}
	if table.MaxSetSize <= 0 || table.MaxSetSize > 65535 || len(table.Words) == 0 || len(table.Words)%table.MaxSetSize != 0 {
		return false
	}
	if table.SymbolCount <= 0 || len(lang.SymbolNames) != table.SymbolCount {
		return false
	}
	for id, wantName := range table.SymbolNames {
		idx := int(id)
		if idx < 0 || idx >= len(lang.SymbolNames) {
			return false
		}
		if lang.SymbolNames[idx] != wantName {
			return false
		}
	}
	numSets := len(table.Words) / table.MaxSetSize
	for i := range lang.LexModes {
		setID := int(lang.LexModes[i].ReservedWordSetID)
		if setID < 0 || setID >= numSets {
			return false
		}
	}
	lang.ReservedWords = table.Words
	lang.MaxReservedWordSetSize = uint16(table.MaxSetSize)
	return true
}

type embeddedLanguageCacheEntry struct {
	blobName   string
	blobSHA256 [32]byte
	lruNode    *list.Element
	lastAccess time.Time
	once       sync.Once
	lang       *gotreesitter.Language
	err        error
}

var (
	embeddedLanguageCacheMu sync.Mutex
	embeddedLanguageCache   = map[string]*embeddedLanguageCacheEntry{}
	embeddedLanguageLRU     list.List
	embeddedLanguageLimit   = -1 // -1 = unlimited

	embeddedLanguageIdleTTL      time.Duration
	embeddedLanguageIdleSweep    = 30 * time.Second
	embeddedLanguageJanitorStop  chan struct{}
	embeddedLanguageJanitorAlive bool
)

// embeddedLanguageEvictionActive is a lock-free mirror of "is any eviction
// configured" (cache limit set OR idle TTL set). The default is no eviction, in
// which case recordEmbeddedLanguageUse's LRU/lastAccess bookkeeping is dead work
// — and that bookkeeping runs on EVERY loadEmbeddedLanguage, which hot external
// scanners (d, html, scss, sql, yaml) trigger per external token. Reading this
// flag instead of locking lets the common path skip the lock + time.Now(). Set
// only from the config setters under embeddedLanguageCacheMu.
var embeddedLanguageEvictionActive atomic.Bool

// recomputeEmbeddedLanguageEvictionActiveLocked refreshes the lock-free flag and
// rebuilds the LRU when transitioning into an eviction-active state, because
// entries loaded while eviction was inactive carry no LRU node (the fast path
// skipped their bookkeeping) and would otherwise be unevictable.
func recomputeEmbeddedLanguageEvictionActiveLocked() {
	active := embeddedLanguageLimit >= 0 || embeddedLanguageIdleTTL > 0
	if active && !embeddedLanguageEvictionActive.Load() {
		for _, entry := range embeddedLanguageCache {
			if entry != nil && entry.lruNode == nil {
				entry.lruNode = embeddedLanguageLRU.PushFront(entry)
			}
		}
	}
	embeddedLanguageEvictionActive.Store(active)
}

func init() {
	if raw := os.Getenv("GOTREESITTER_GRAMMAR_CACHE_LIMIT"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err == nil {
			SetEmbeddedLanguageCacheLimit(limit)
		}
	}
	if raw := os.Getenv("GOTREESITTER_GRAMMAR_IDLE_SWEEP"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			SetEmbeddedLanguageIdleSweepInterval(d)
		}
	}
	if raw := os.Getenv("GOTREESITTER_GRAMMAR_IDLE_TTL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			SetEmbeddedLanguageIdleTTL(d)
		}
	}
}

func loadEmbeddedLanguage(blobName string) *gotreesitter.Language {
	name := strings.TrimSuffix(blobName, ".bin")
	if name != "" && name != blobName {
		if lang := loadPreferredLanguageOverride(name); lang != nil {
			return lang
		}
	}
	return loadEmbeddedLanguageBase(blobName)
}

// LoadLanguage deserializes a raw grammar blob and attaches registered
// gotreesitter/grammars runtime support for name, including external scanners,
// external lex-state tables, and any runtime profile certified for this exact
// blob. Use this instead of gotreesitter.LoadLanguage when loading blobs that
// came from BlobByName, grammar_subset, or a remote WASM bundle and the caller
// knows the language name.
func LoadLanguage(name string, data []byte) (*gotreesitter.Language, error) {
	lang, err := gotreesitter.LoadLanguage(data)
	if err != nil {
		return nil, err
	}
	AttachLanguageSupport(name, lang)
	attachBuiltinLanguageRuntimeProfile(name, sha256.Sum256(data), lang)
	return lang, nil
}

// AttachLanguageSupport attaches registered scanner and external lex-state
// support for name to lang. Certified runtime profiles require the original
// blob identity and are attached by LoadLanguage or the embedded loader.
func AttachLanguageSupport(name string, lang *gotreesitter.Language) bool {
	if lang == nil {
		return false
	}
	lookupName := canonicalLanguageName(name)
	if lookupName == "" {
		lookupName = canonicalLanguageName(lang.Name)
	}
	if lang.Name == "" {
		lang.Name = lookupName
	}
	return AdaptScannerForLanguage(lookupName, lang)
}

func canonicalLanguageName(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".bin")
	if name == "" {
		return ""
	}
	blobSources.RLock()
	canonical := blobSources.canonical
	blobSources.RUnlock()
	if canonical != nil {
		if resolved := canonical(name); resolved != "" {
			return resolved
		}
	}
	return strings.ToLower(name)
}

func loadEmbeddedLanguageBase(blobName string) *gotreesitter.Language {
	entry := getEmbeddedLanguageCacheEntry(blobName)
	entry.once.Do(func() {
		entry.lang, entry.blobSHA256, entry.err = decodeEmbeddedLanguage(blobName)
		if entry.err == nil {
			// Attach external scanner, lex states, and certified runtime profile
			// if registered.
			name := strings.TrimSuffix(blobName, ".bin")
			attachExternalScannerForLanguage(name, entry.lang)
			attachRegisteredExternalLexStates(name, entry.lang)
			attachBuiltinLanguageRuntimeProfile(name, entry.blobSHA256, entry.lang)
		}
	})
	if entry.err != nil {
		panic(fmt.Sprintf("gotreesitter: failed to load grammar %q: %v", blobName, entry.err))
	}
	recordEmbeddedLanguageUse(entry)
	return entry.lang
}

func getEmbeddedLanguageCacheEntry(blobName string) *embeddedLanguageCacheEntry {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()
	if entry, ok := embeddedLanguageCache[blobName]; ok {
		return entry
	}
	entry := &embeddedLanguageCacheEntry{blobName: blobName}
	embeddedLanguageCache[blobName] = entry
	return entry
}

func recordEmbeddedLanguageUse(entry *embeddedLanguageCacheEntry) {
	// Fast path: with no cache limit and no idle TTL (the default), nothing below
	// is ever consulted — no eviction can occur and getEmbeddedLanguageCacheEntry
	// already inserted the entry — so the lock + time.Now() + LRU bookkeeping are
	// pure overhead. Hot external scanners re-fetch their Language per token, so
	// this runs on the tokenization hot path.
	if !embeddedLanguageEvictionActive.Load() {
		return
	}

	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()

	now := time.Now()
	entry.lastAccess = now

	if embeddedLanguageLimit == 0 {
		removeEmbeddedLanguageEntryLocked(entry)
		return
	}

	if _, ok := embeddedLanguageCache[entry.blobName]; !ok {
		embeddedLanguageCache[entry.blobName] = entry
	}
	if entry.lruNode != nil {
		embeddedLanguageLRU.MoveToFront(entry.lruNode)
	} else {
		entry.lruNode = embeddedLanguageLRU.PushFront(entry)
	}

	enforceEmbeddedLanguageLimitLocked()
	evictIdleEmbeddedLanguagesLocked(now)
}

func removeEmbeddedLanguageEntryLocked(entry *embeddedLanguageCacheEntry) {
	if entry == nil {
		return
	}
	delete(embeddedLanguageCache, entry.blobName)
	if entry.lruNode != nil {
		embeddedLanguageLRU.Remove(entry.lruNode)
		entry.lruNode = nil
	}
}

func enforceEmbeddedLanguageLimitLocked() {
	if embeddedLanguageLimit < 0 {
		return
	}
	for len(embeddedLanguageCache) > embeddedLanguageLimit {
		tail := embeddedLanguageLRU.Back()
		if tail == nil {
			return
		}
		entry, ok := tail.Value.(*embeddedLanguageCacheEntry)
		if !ok || entry == nil {
			embeddedLanguageLRU.Remove(tail)
			continue
		}
		removeEmbeddedLanguageEntryLocked(entry)
	}
}

func evictIdleEmbeddedLanguagesLocked(now time.Time) {
	if embeddedLanguageIdleTTL <= 0 {
		return
	}
	for _, entry := range embeddedLanguageCache {
		if entry == nil || entry.lastAccess.IsZero() {
			continue
		}
		if now.Sub(entry.lastAccess) > embeddedLanguageIdleTTL {
			removeEmbeddedLanguageEntryLocked(entry)
		}
	}
}

// SetEmbeddedLanguageCacheLimit sets the maximum number of decoded grammar
// blobs retained in the in-process cache.
//
// - limit < 0: unlimited cache size (default)
// - limit == 0: disable cache retention (decode on each call)
// - limit > 0: retain at most limit most recently used grammars
func SetEmbeddedLanguageCacheLimit(limit int) {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()

	if limit < 0 {
		embeddedLanguageLimit = -1
		recomputeEmbeddedLanguageEvictionActiveLocked()
		return
	}
	embeddedLanguageLimit = limit
	recomputeEmbeddedLanguageEvictionActiveLocked()
	enforceEmbeddedLanguageLimitLocked()
	evictIdleEmbeddedLanguagesLocked(time.Now())
}

// EmbeddedLanguageCacheStats returns the current decoded-grammar cache size and
// configured cache limit.
func EmbeddedLanguageCacheStats() (loaded int, limit int) {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()
	return len(embeddedLanguageCache), embeddedLanguageLimit
}

// UnloadEmbeddedLanguage removes one grammar blob from the decoded cache.
// Existing parser instances that already reference the language remain valid.
func UnloadEmbeddedLanguage(blobName string) bool {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()

	entry, ok := embeddedLanguageCache[blobName]
	if !ok {
		return false
	}
	removeEmbeddedLanguageEntryLocked(entry)
	return true
}

// PurgeEmbeddedLanguageCache removes all decoded grammar blobs from cache and
// returns the number of removed entries.
func PurgeEmbeddedLanguageCache() int {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()

	count := len(embeddedLanguageCache)
	embeddedLanguageCache = map[string]*embeddedLanguageCacheEntry{}
	embeddedLanguageLRU.Init()
	purgePreferredLanguageOverrideCache()
	return count
}

// SetEmbeddedLanguageIdleTTL controls idle-time eviction for decoded grammars.
// A value <= 0 disables idle eviction.
func SetEmbeddedLanguageIdleTTL(ttl time.Duration) {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()

	embeddedLanguageIdleTTL = ttl
	if ttl <= 0 {
		stopEmbeddedLanguageJanitorLocked()
		recomputeEmbeddedLanguageEvictionActiveLocked()
		return
	}
	if embeddedLanguageIdleSweep <= 0 {
		embeddedLanguageIdleSweep = 30 * time.Second
	}
	recomputeEmbeddedLanguageEvictionActiveLocked()
	startEmbeddedLanguageJanitorLocked()
	evictIdleEmbeddedLanguagesLocked(time.Now())
}

// SetEmbeddedLanguageIdleSweepInterval controls how often idle cache entries
// are checked when idle eviction is enabled.
func SetEmbeddedLanguageIdleSweepInterval(interval time.Duration) {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()

	if interval <= 0 {
		return
	}
	embeddedLanguageIdleSweep = interval
	if embeddedLanguageJanitorAlive {
		stopEmbeddedLanguageJanitorLocked()
		if embeddedLanguageIdleTTL > 0 {
			startEmbeddedLanguageJanitorLocked()
		}
	}
}

// EmbeddedLanguageIdleConfig returns the current idle eviction settings.
func EmbeddedLanguageIdleConfig() (ttl time.Duration, sweepInterval time.Duration) {
	embeddedLanguageCacheMu.Lock()
	defer embeddedLanguageCacheMu.Unlock()
	return embeddedLanguageIdleTTL, embeddedLanguageIdleSweep
}

func startEmbeddedLanguageJanitorLocked() {
	if embeddedLanguageJanitorAlive || embeddedLanguageIdleTTL <= 0 {
		return
	}
	if embeddedLanguageIdleSweep <= 0 {
		embeddedLanguageIdleSweep = 30 * time.Second
	}
	stop := make(chan struct{})
	embeddedLanguageJanitorStop = stop
	embeddedLanguageJanitorAlive = true
	sweep := embeddedLanguageIdleSweep

	go func() {
		ticker := time.NewTicker(sweep)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				embeddedLanguageCacheMu.Lock()
				evictIdleEmbeddedLanguagesLocked(time.Now())
				embeddedLanguageCacheMu.Unlock()
			case <-stop:
				return
			}
		}
	}()
}

func stopEmbeddedLanguageJanitorLocked() {
	if !embeddedLanguageJanitorAlive {
		return
	}
	close(embeddedLanguageJanitorStop)
	embeddedLanguageJanitorStop = nil
	embeddedLanguageJanitorAlive = false
}

// LookupExternalScanner returns the registered hand-written external scanner
// for the given language name (e.g. "python"), or nil if none is registered.
func LookupExternalScanner(name string) gotreesitter.ExternalScanner {
	return externalScannerRegistry[name]
}

// LookupExternalLexStates returns the registered external lex states table
// for the given language name, or nil if none is registered.
func LookupExternalLexStates(name string) [][]bool {
	return externalLexStatesRegistry[name]
}

type languageBoundExternalScanner interface {
	ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner
}

func attachExternalScannerForLanguage(name string, lang *gotreesitter.Language) bool {
	s, ok := externalScannerRegistry[name]
	if !ok || lang == nil {
		return false
	}
	if bound, ok := s.(languageBoundExternalScanner); ok {
		lang.ExternalScanner = bound.ExternalScannerForLanguage(lang)
	} else {
		lang.ExternalScanner = s
	}
	return lang.ExternalScanner != nil
}

// AdaptScannerForLanguage adapts the registered hand-written scanner for the
// named language to work with a different Language (e.g., one produced by
// grammargen). It loads the ts2go reference Language to get the scanner's
// native Symbol IDs, then builds an adapter that remaps them to the target
// Language's Symbol IDs.
func AdaptScannerForLanguage(name string, targetLang *gotreesitter.Language) bool {
	if targetLang == nil || len(targetLang.ExternalSymbols) == 0 {
		return false
	}
	if name == "" {
		return false
	}

	lookupName := name
	if _, ok := externalScannerRegistry[lookupName]; !ok {
		lowerName := strings.ToLower(name)
		if _, ok := externalScannerRegistry[lowerName]; ok {
			lookupName = lowerName
		}
	}
	if _, ok := externalScannerRegistry[lookupName]; !ok {
		return false
	}
	if s, ok := externalScannerRegistry[lookupName]; ok {
		if bound, ok := s.(languageBoundExternalScanner); ok {
			targetLang.ExternalScanner = bound.ExternalScannerForLanguage(targetLang)
		}
	}
	if targetLang.ExternalScanner != nil {
		attachRegisteredExternalLexStates(lookupName, targetLang)
		return true
	}

	// Scanner adaptation needs the checked-in ts2go blob as the stable symbol
	// oracle. Do not route this through override lookup or we can recurse back
	// into the same override currently being decoded.
	refLang := loadEmbeddedLanguageBase(lookupName + ".bin")
	if refLang == nil || refLang.ExternalScanner == nil {
		return false
	}

	if len(refLang.ExternalSymbols) == len(targetLang.ExternalSymbols) {
		same := true
		for i := range refLang.ExternalSymbols {
			if refLang.ExternalSymbols[i] != targetLang.ExternalSymbols[i] {
				same = false
				break
			}
		}
		if same {
			targetLang.ExternalScanner = refLang.ExternalScanner
			attachRegisteredExternalLexStates(lookupName, targetLang)
			return true
		}
	}

	adapted, ok := gotreesitter.AdaptExternalScannerByExternalOrder(refLang, targetLang)
	if !ok {
		return false
	}
	targetLang.ExternalScanner = adapted
	attachRegisteredExternalLexStates(lookupName, targetLang)
	return true
}

func attachRegisteredExternalLexStates(name string, targetLang *gotreesitter.Language) {
	if targetLang == nil {
		return
	}
	els := externalLexStatesRegistry[name]
	if len(els) > 0 && len(targetLang.ExternalLexStates) == 0 {
		targetLang.ExternalLexStates = els
	}
	gotreesitter.CertifyCRecoveryCostCompetition(targetLang)
}

func decodeEmbeddedLanguage(blobName string) (*gotreesitter.Language, [32]byte, error) {
	blob, err := readGrammarBlob(blobName)
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("read grammar blob %q: %w", blobName, err)
	}
	defer blob.close()

	sum := sha256.Sum256(blob.data)
	lang, err := decodeLanguageBlobData(blobName, blob.data)
	return lang, sum, err
}

func decodeLanguageBlobData(blobName string, data []byte) (*gotreesitter.Language, error) {
	lang, err := gotreesitter.LoadLanguage(data)
	if err != nil {
		return nil, fmt.Errorf("decode grammar blob %q: %w", blobName, err)
	}
	compactDecodedLanguage(lang)
	repairNoLookaheadLexModes(lang)
	// Attach reserved words before any repair below that appends a
	// synthetic symbol name (e.g. the JS/TS optional-chain repair): the
	// sidecar's recorded symbol count is the original ts_symbol_names
	// count from parser.c, and appended synthetic symbols never appear in
	// any reserved-word set, so validating first keeps the exact symbol
	// count match meaningful.
	attachReservedWordsForBlob(blobName, lang)
	repairJavaScriptTypeScriptOptionalChainTokenSymbol(blobName, lang)
	repairDartCollapsedLeafTokenSymbols(blobName, lang)
	repairDhallUnicodeAnonymousSymbolNames(blobName, lang)
	attachReduceChainHints(blobName, lang)

	return lang, nil
}

// attachReservedWordsForBlob normalizes blobName to a bare language name
// (stripping any directory and .bin suffix, matching the sibling repair*
// helpers above) and, when a sidecar reserved-word table is registered for
// that name, attaches it via attachRegisteredReservedWords. This runs for
// every decoded language, not only ones with a hand-written external
// scanner, because reserved words are a plain grammar-table gap: several
// checked-in blobs predate cmd/ts2go's reserved-word extraction.
func attachReservedWordsForBlob(blobName string, lang *gotreesitter.Language) {
	if lang == nil {
		return
	}
	name := strings.TrimSuffix(blobName, ".bin")
	if slash := strings.LastIndexAny(name, "/\\"); slash >= 0 {
		name = name[slash+1:]
	}
	if name == "" {
		return
	}
	attachRegisteredReservedWords(name, lang)
}

func repairDhallUnicodeAnonymousSymbolNames(blobName string, lang *gotreesitter.Language) {
	if lang == nil {
		return
	}
	name := strings.TrimSuffix(blobName, ".bin")
	if slash := strings.LastIndexAny(name, "/\\"); slash >= 0 {
		name = name[slash+1:]
	}
	if name != "dhall" {
		return
	}
	for i := range lang.SymbolNames {
		if i >= len(lang.SymbolMetadata) {
			break
		}
		meta := lang.SymbolMetadata[i]
		if !meta.Visible || meta.Named || !strings.Contains(lang.SymbolNames[i], `\u`) {
			continue
		}
		normalized := decodeDhallAnonymousUnicodeEscapes(lang.SymbolNames[i])
		if normalized == lang.SymbolNames[i] {
			continue
		}
		lang.SymbolNames[i] = normalized
		lang.SymbolMetadata[i].Name = normalized
	}
}

func decodeDhallAnonymousUnicodeEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, next, ok := parseDhallAnonymousUnicodeEscape(s, i)
		if ok {
			b.WriteRune(r)
			i = next
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func parseDhallAnonymousUnicodeEscape(s string, i int) (rune, int, bool) {
	if i+2 > len(s) || s[i] != '\\' || s[i+1] != 'u' {
		return 0, i, false
	}
	if i+2 < len(s) && s[i+2] == '{' {
		end := strings.IndexByte(s[i+3:], '}')
		if end < 0 {
			return 0, i, false
		}
		hex := s[i+3 : i+3+end]
		if hex == "" {
			return 0, i, false
		}
		r, ok := parseDhallAnonymousUnicodeHex(hex)
		if !ok || utf16.IsSurrogate(r) {
			return 0, i, false
		}
		return r, i + 3 + end + 1, true
	}
	if i+6 > len(s) {
		return 0, i, false
	}
	r, ok := parseDhallAnonymousUnicodeHex(s[i+2 : i+6])
	if !ok {
		return 0, i, false
	}
	if utf16.IsSurrogate(r) {
		if r < 0xD800 || r > 0xDBFF || i+12 > len(s) || s[i+6] != '\\' || s[i+7] != 'u' {
			return 0, i, false
		}
		r2, ok := parseDhallAnonymousUnicodeHex(s[i+8 : i+12])
		if !ok || r2 < 0xDC00 || r2 > 0xDFFF {
			return 0, i, false
		}
		return utf16.DecodeRune(r, r2), i + 12, true
	}
	return r, i + 6, true
}

func parseDhallAnonymousUnicodeHex(hex string) (rune, bool) {
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, false
	}
	r := rune(n)
	if !utf8.ValidRune(r) && !utf16.IsSurrogate(r) {
		return 0, false
	}
	return r, true
}

func repairJavaScriptTypeScriptOptionalChainTokenSymbol(blobName string, lang *gotreesitter.Language) {
	if lang == nil {
		return
	}
	name := strings.TrimSuffix(blobName, ".bin")
	if slash := strings.LastIndexAny(name, "/\\"); slash >= 0 {
		name = name[slash+1:]
	}
	switch name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if !embeddedLanguageHasSymbolName(lang, "optional_chain") || embeddedLanguageHasSymbolName(lang, "?.") {
		return
	}
	for len(lang.SymbolMetadata) < len(lang.SymbolNames) {
		lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{})
	}
	lang.SymbolNames = append(lang.SymbolNames, "?.")
	lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{
		Name:    "?.",
		Visible: true,
		Named:   false,
	})
}

func repairDartCollapsedLeafTokenSymbols(blobName string, lang *gotreesitter.Language) {
	if lang == nil {
		return
	}
	name := strings.TrimSuffix(blobName, ".bin")
	if slash := strings.LastIndexAny(name, "/\\"); slash >= 0 {
		name = name[slash+1:]
	}
	if name != "dart" {
		return
	}
	repairCollapsedLeafTokenSymbol(lang, "nullable_type", "?")
	repairCollapsedLeafTokenSymbol(lang, "null_literal", "null")
}

func repairCollapsedLeafTokenSymbol(lang *gotreesitter.Language, parentName, childName string) {
	if !embeddedLanguageHasSymbolName(lang, parentName) || embeddedLanguageHasSymbolName(lang, childName) {
		return
	}
	for len(lang.SymbolMetadata) < len(lang.SymbolNames) {
		lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{})
	}
	lang.SymbolNames = append(lang.SymbolNames, childName)
	lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{
		Name:    childName,
		Visible: true,
		Named:   false,
	})
}

func attachReduceChainHints(blobName string, lang *gotreesitter.Language) {
	if lang == nil || len(lang.ReduceChainHints) != 0 {
		return
	}
	name := strings.TrimSuffix(blobName, ".bin")
	if slash := strings.LastIndexAny(name, "/\\"); slash >= 0 {
		name = name[slash+1:]
	}
	switch name {
	case "python":
		if !embeddedLanguageSymbolNameMatches(lang, gotreesitter.Symbol(101), "_newline") {
			return
		}
		lang.ReduceChainHints = []gotreesitter.ReduceChainHint{{
			StartState:     gotreesitter.StateID(1101),
			Lookahead:      gotreesitter.Symbol(101),
			TerminalStates: []gotreesitter.StateID{gotreesitter.StateID(2336), gotreesitter.StateID(2361), gotreesitter.StateID(2098), gotreesitter.StateID(2460)},
			TerminalAction: gotreesitter.ReduceChainTerminalSingleShift,
			MaxSteps:       10,
		}}
	case "rust":
		if !embeddedLanguageSymbolNameMatches(lang, gotreesitter.Symbol(5), ")") {
			return
		}
		lang.ReduceChainHints = []gotreesitter.ReduceChainHint{{
			StartState:     gotreesitter.StateID(205),
			Lookahead:      gotreesitter.Symbol(5),
			TerminalStates: []gotreesitter.StateID{gotreesitter.StateID(98), gotreesitter.StateID(132), gotreesitter.StateID(133)},
			TerminalAction: gotreesitter.ReduceChainTerminalSingleShift,
			MaxSteps:       32,
		}}
	}
}

func embeddedLanguageSymbolNameMatches(lang *gotreesitter.Language, sym gotreesitter.Symbol, name string) bool {
	idx := int(sym)
	return idx >= 0 && idx < len(lang.SymbolNames) && lang.SymbolNames[idx] == name
}

func embeddedLanguageHasSymbolName(lang *gotreesitter.Language, name string) bool {
	if lang == nil {
		return false
	}
	for _, symbolName := range lang.SymbolNames {
		if symbolName == name {
			return true
		}
	}
	return false
}

func decodeLanguageBlobFromPath(path string) (*gotreesitter.Language, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read grammar blob %q: %w", path, err)
	}
	return decodeLanguageBlobData(path, data)
}

func repairNoLookaheadLexModes(lang *gotreesitter.Language) {
	if lang == nil || len(lang.LexModes) == 0 || len(lang.ParseActions) == 0 {
		return
	}
	for state := 0; state < len(lang.LexModes) && state < int(lang.StateCount); state++ {
		if lang.LexModes[state].LexStateIndex() == ^uint32(0) {
			continue
		}
		eofIdx := grammarLookupActionIndex(lang, gotreesitter.StateID(state), 0)
		if eofIdx == 0 || int(eofIdx) >= len(lang.ParseActions) {
			continue
		}
		eofEntry := lang.ParseActions[eofIdx]
		if len(eofEntry.Actions) == 0 {
			continue
		}
		allReduce := true
		for _, act := range eofEntry.Actions {
			if act.Type != gotreesitter.ParseActionReduce {
				allReduce = false
				break
			}
		}
		if !allReduce {
			continue
		}

		onlyEOF := true
		for sym := gotreesitter.Symbol(1); uint32(sym) < lang.TokenCount; sym++ {
			if grammarLookupActionIndex(lang, gotreesitter.StateID(state), sym) != 0 {
				onlyEOF = false
				break
			}
		}
		if onlyEOF {
			lang.LexModes[state].SetLexStateIndex(^uint32(0))
		}
	}
}

func grammarLookupActionIndex(lang *gotreesitter.Language, state gotreesitter.StateID, sym gotreesitter.Symbol) uint16 {
	if lang == nil {
		return 0
	}
	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	if int(state) < denseLimit {
		if int(state) >= len(lang.ParseTable) {
			return 0
		}
		row := lang.ParseTable[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}

	smallIdx := int(state) - int(lang.LargeStateCount)
	if smallIdx < 0 || smallIdx >= len(lang.SmallParseTableMap) {
		return 0
	}
	table := lang.SmallParseTable
	offset := lang.SmallParseTableMap[smallIdx]
	if int(offset) >= len(table) {
		return 0
	}
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}
