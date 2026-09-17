package grammarruntime

import (
	"hash/fnv"
	"os"
	"strconv"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

var (
	grammarCompactEnabled = true

	stringInternLimit = 200000
	stringInternMu    sync.Mutex
	stringInternPool  = map[string]string{}

	transitionInternLimit = 20000
	transitionInternMu    sync.Mutex
	transitionInternPool  = map[uint64][][]gotreesitter.LexTransition{}
)

func init() {
	if raw := os.Getenv("GOTREESITTER_GRAMMAR_COMPACT"); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			grammarCompactEnabled = v
		}
	}
	if raw := os.Getenv("GOTREESITTER_GRAMMAR_STRING_INTERN_LIMIT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			stringInternLimit = n
		}
	}
	if raw := os.Getenv("GOTREESITTER_GRAMMAR_TRANSITION_INTERN_LIMIT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			transitionInternLimit = n
		}
	}
}

func compactDecodedLanguage(lang *gotreesitter.Language) {
	if !grammarCompactEnabled || lang == nil {
		return
	}

	for i := range lang.SymbolNames {
		lang.SymbolNames[i] = internGrammarString(lang.SymbolNames[i])
	}
	for i := range lang.FieldNames {
		lang.FieldNames[i] = internGrammarString(lang.FieldNames[i])
	}
	for i := range lang.SymbolMetadata {
		lang.SymbolMetadata[i].Name = internGrammarString(lang.SymbolMetadata[i].Name)
	}

	for i := range lang.LexStates {
		lang.LexStates[i].Transitions = compactAndInternTransitions(lang.LexStates[i].Transitions)
	}
	for i := range lang.KeywordLexStates {
		lang.KeywordLexStates[i].Transitions = compactAndInternTransitions(lang.KeywordLexStates[i].Transitions)
	}
}

func internGrammarString(s string) string {
	if s == "" || stringInternLimit == 0 {
		return s
	}

	stringInternMu.Lock()
	defer stringInternMu.Unlock()

	if interned, ok := stringInternPool[s]; ok {
		return interned
	}
	if stringInternLimit > 0 && len(stringInternPool) >= stringInternLimit {
		return s
	}
	stringInternPool[s] = s
	return s
}

func compactAndInternTransitions(ts []gotreesitter.LexTransition) []gotreesitter.LexTransition {
	if len(ts) == 0 {
		return ts
	}

	// Merge adjacent ranges with identical target behavior.
	merged := make([]gotreesitter.LexTransition, 0, len(ts))
	for _, tr := range ts {
		n := len(merged)
		if n > 0 {
			prev := merged[n-1]
			if prev.NextState == tr.NextState &&
				prev.Skip == tr.Skip &&
				prev.Hi < tr.Lo &&
				prev.Hi+1 == tr.Lo {
				merged[n-1].Hi = tr.Hi
				continue
			}
		}
		merged = append(merged, tr)
	}
	if len(merged) == 0 {
		return merged
	}

	if transitionInternLimit == 0 {
		return merged
	}

	hash := hashTransitions(merged)

	transitionInternMu.Lock()
	defer transitionInternMu.Unlock()

	if bucket, ok := transitionInternPool[hash]; ok {
		for _, existing := range bucket {
			if lexTransitionsEqual(existing, merged) {
				return existing
			}
		}
	}

	if transitionInternLimit > 0 && len(transitionInternPool) >= transitionInternLimit {
		return merged
	}

	canonical := append([]gotreesitter.LexTransition(nil), merged...)
	transitionInternPool[hash] = append(transitionInternPool[hash], canonical)
	return canonical
}

func lexTransitionsEqual(a []gotreesitter.LexTransition, b []gotreesitter.LexTransition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Lo != b[i].Lo || a[i].Hi != b[i].Hi || a[i].NextState != b[i].NextState || a[i].Skip != b[i].Skip {
			return false
		}
	}
	return true
}

func hashTransitions(ts []gotreesitter.LexTransition) uint64 {
	h := fnv.New64a()
	var buf [24]byte
	for _, tr := range ts {
		putUint64(buf[0:8], uint64(tr.Lo))
		putUint64(buf[8:16], uint64(tr.Hi))
		putUint32(buf[16:20], uint32(tr.NextState))
		if tr.Skip {
			buf[20] = 1
		} else {
			buf[20] = 0
		}
		_, _ = h.Write(buf[:21])
	}
	return h.Sum64()
}

func putUint64(dst []byte, v uint64) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
	dst[4] = byte(v >> 32)
	dst[5] = byte(v >> 40)
	dst[6] = byte(v >> 48)
	dst[7] = byte(v >> 56)
}

func putUint32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}
