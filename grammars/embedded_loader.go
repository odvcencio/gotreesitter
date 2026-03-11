package grammars

import (
	"bytes"
	"compress/gzip"
	"container/list"
	"encoding/gob"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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

type embeddedLanguageCacheEntry struct {
	blobName   string
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
	entry := getEmbeddedLanguageCacheEntry(blobName)
	entry.once.Do(func() {
		entry.lang, entry.err = decodeEmbeddedLanguage(blobName)
		if entry.err == nil {
			// Attach external scanner and lex states if registered.
			name := strings.TrimSuffix(blobName, ".bin")
			if s, ok := externalScannerRegistry[name]; ok {
				entry.lang.ExternalScanner = s
			}
			if els, ok := externalLexStatesRegistry[name]; ok {
				entry.lang.ExternalLexStates = els
			}
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
		return
	}
	embeddedLanguageLimit = limit
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
		return
	}
	if embeddedLanguageIdleSweep <= 0 {
		embeddedLanguageIdleSweep = 30 * time.Second
	}
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

// AdaptScannerForLanguage adapts the registered hand-written scanner for the
// named language to work with a different Language (e.g., one produced by
// grammargen). It loads the ts2go reference Language to get the scanner's
// native Symbol IDs, then builds an adapter that remaps them to the target
// Language's Symbol IDs.
func AdaptScannerForLanguage(name string, targetLang *gotreesitter.Language) bool {
	if targetLang == nil || len(targetLang.ExternalSymbols) == 0 {
		return false
	}

	refLang := loadEmbeddedLanguage(name + ".bin")
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
			if els := externalLexStatesRegistry[name]; els != nil {
				targetLang.ExternalLexStates = els
			}
			return true
		}
	}

	adapted, ok := gotreesitter.AdaptExternalScannerByExternalOrder(refLang, targetLang)
	if !ok {
		return false
	}
	targetLang.ExternalScanner = adapted
	if els := externalLexStatesRegistry[name]; els != nil {
		targetLang.ExternalLexStates = els
	}
	return true
}

func decodeEmbeddedLanguage(blobName string) (*gotreesitter.Language, error) {
	blob, err := readGrammarBlob(blobName)
	if err != nil {
		return nil, fmt.Errorf("read grammar blob %q: %w", blobName, err)
	}
	defer blob.close()

	gzr, err := gzip.NewReader(bytes.NewReader(blob.data))
	if err != nil {
		return nil, fmt.Errorf("open gzip grammar blob %q: %w", blobName, err)
	}
	defer gzr.Close()

	dec := gob.NewDecoder(gzr)
	var lang gotreesitter.Language
	if err := dec.Decode(&lang); err != nil {
		return nil, fmt.Errorf("decode grammar blob %q: %w", blobName, err)
	}

	compactDecodedLanguage(&lang)
	repairNoLookaheadLexModes(&lang)

	return &lang, nil
}

func repairNoLookaheadLexModes(lang *gotreesitter.Language) {
	if lang == nil || len(lang.LexModes) == 0 || len(lang.ParseActions) == 0 {
		return
	}
	for state := 0; state < len(lang.LexModes) && state < int(lang.StateCount); state++ {
		if lang.LexModes[state].LexState == ^uint16(0) {
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
			lang.LexModes[state].LexState = ^uint16(0)
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
