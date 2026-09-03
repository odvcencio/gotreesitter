//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

var errSelectedStoreQueryMissingUnsupported = errors.New("selected-store query route does not support MISSING patterns")

type selectedStoreQueryBuffer struct {
	matches  []queryReaderMatch[queryValueCapture[core.SelectedNodeID]]
	worklist []queryReaderWorkItem[core.SelectedNodeID]
	ranges   []HighlightRange
	resolved []HighlightRange
}

type selectedStoreQueryReader struct {
	store *core.SelectedStore
}

func (r selectedStoreQueryReader) IsNil(id core.SelectedNodeID) bool { return id == 0 }

func (r selectedStoreQueryReader) Symbol(id core.SelectedNodeID) Symbol {
	if r.store == nil {
		return 0
	}
	symbol, _ := r.store.NodeSymbol(id)
	return Symbol(symbol)
}

func (r selectedStoreQueryReader) IsNamed(id core.SelectedNodeID) bool {
	if r.store == nil {
		return false
	}
	named, _ := r.store.NodeNamed(id)
	return named
}

func (r selectedStoreQueryReader) IsMissing(id core.SelectedNodeID) bool {
	if r.store == nil {
		return false
	}
	record, ok := r.store.Record(id)
	return ok && record.Missing()
}

func (r selectedStoreQueryReader) Type(id core.SelectedNodeID, lang *Language) string {
	symbol := r.Symbol(id)
	if symbol == errorSymbol {
		return "ERROR"
	}
	if lang != nil && int(symbol) < len(lang.SymbolNames) {
		return unescapePunctuationSymbolName(lang.SymbolNames[symbol])
	}
	return ""
}

func (r selectedStoreQueryReader) StartByte(id core.SelectedNodeID) uint32 {
	if r.store == nil {
		return 0
	}
	start, _, _ := r.store.NodeRange(id)
	return start
}

func (r selectedStoreQueryReader) EndByte(id core.SelectedNodeID) uint32 {
	if r.store == nil {
		return 0
	}
	_, end, _ := r.store.NodeRange(id)
	return end
}

func (r selectedStoreQueryReader) Text(id core.SelectedNodeID, source []byte) string {
	if r.store == nil {
		return ""
	}
	start, end, ok := r.store.NodeRange(id)
	if !ok || end < start || uint64(end) > uint64(len(source)) {
		return ""
	}
	return string(source[start:end])
}

func (r selectedStoreQueryReader) Parent(id core.SelectedNodeID) (core.SelectedNodeID, bool) {
	if r.store == nil {
		return 0, false
	}
	return r.store.NodeParent(id)
}

func (r selectedStoreQueryReader) ChildCount(id core.SelectedNodeID) int {
	if r.store == nil {
		return 0
	}
	count, _ := r.store.NodeChildCount(id)
	return int(count)
}

func (r selectedStoreQueryReader) Child(id core.SelectedNodeID, index int) (core.SelectedNodeID, bool) {
	if r.store == nil || index < 0 {
		return 0, false
	}
	return r.store.ChildID(id, uint32(index))
}

func (r selectedStoreQueryReader) ChildNamed(parent core.SelectedNodeID, index int) (bool, bool) {
	child, ok := r.Child(parent, index)
	if !ok {
		return false, false
	}
	return r.store.NodeNamed(child)
}

func (r selectedStoreQueryReader) ChildCanMatch(q *Query, step *QueryStep, parent core.SelectedNodeID, index int, lang *Language) bool {
	child, ok := r.Child(parent, index)
	return ok && nodeCanMatchStepShallowWithReader(q, step, child, lang, r)
}

func (r selectedStoreQueryReader) ChildHasField(parent core.SelectedNodeID, index int, field FieldID, _ *Language) bool {
	if field == 0 {
		return true
	}
	child, ok := r.Child(parent, index)
	if !ok {
		return false
	}
	childField, ok := r.store.NodeField(child)
	return ok && FieldID(childField) == field
}

func (r selectedStoreQueryReader) HasChildWithField(parent core.SelectedNodeID, field FieldID, lang *Language) bool {
	for index := 0; index < r.ChildCount(parent); index++ {
		if r.ChildHasField(parent, index, field, lang) {
			return true
		}
	}
	return false
}

func (selectedStoreQueryReader) NewCapture(name string, id core.SelectedNodeID) queryValueCapture[core.SelectedNodeID] {
	return queryValueCapture[core.SelectedNodeID]{Name: name, Node: id}
}

func (selectedStoreQueryReader) CaptureName(capture queryValueCapture[core.SelectedNodeID]) string {
	return capture.Name
}

func (selectedStoreQueryReader) CaptureNode(capture queryValueCapture[core.SelectedNodeID]) core.SelectedNodeID {
	return capture.Node
}

func (selectedStoreQueryReader) CaptureTextOverride(capture queryValueCapture[core.SelectedNodeID]) string {
	return capture.TextOverride
}

func (selectedStoreQueryReader) SetCaptureTextOverride(capture *queryValueCapture[core.SelectedNodeID], text string) {
	capture.TextOverride = text
}

func selectedStoreQuerySupports(q *Query) bool {
	if q == nil {
		return false
	}
	for patternIndex := range q.patterns {
		if queryStepsRequireMissing(q.patterns[patternIndex].steps) {
			return false
		}
	}
	return true
}

func executeSelectedStoreQuery(
	q *Query,
	store *core.SelectedStore,
	lang *Language,
	source []byte,
	buffer *selectedStoreQueryBuffer,
) ([]queryReaderMatch[queryValueCapture[core.SelectedNodeID]], error) {
	if !selectedStoreQuerySupports(q) {
		return nil, errSelectedStoreQueryMissingUnsupported
	}
	if store == nil || lang == nil || store.Root() == 0 {
		return nil, nil
	}
	if buffer == nil {
		buffer = &selectedStoreQueryBuffer{}
	}
	reader := selectedStoreQueryReader{store: store}
	buffer.matches, buffer.worklist = executeQueryWithReader(
		q,
		store.Root(),
		lang,
		source,
		reader,
		buffer.matches[:0],
		buffer.worklist[:0],
	)
	return buffer.matches, nil
}

func highlightSelectedStore(
	q *Query,
	store *core.SelectedStore,
	lang *Language,
	source []byte,
	buffer *selectedStoreQueryBuffer,
) ([]HighlightRange, error) {
	if buffer == nil {
		buffer = &selectedStoreQueryBuffer{}
	}
	matches, err := executeSelectedStoreQuery(q, store, lang, source, buffer)
	if err != nil {
		return nil, err
	}
	ranges := buffer.ranges[:0]
	if cap(ranges) < len(matches) {
		ranges = make([]HighlightRange, 0, len(matches))
	}
	for _, match := range matches {
		for _, capture := range match.Captures {
			start, end, ok := store.NodeRange(capture.Node)
			if !ok || start == end {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte:    start,
				EndByte:      end,
				Capture:      capture.Name,
				PatternIndex: match.PatternIndex,
			})
		}
	}
	buffer.ranges = ranges[:0]
	if len(ranges) == 0 {
		return nil, nil
	}
	resolved := resolveOverlapsInto(ranges, buffer.resolved[:0])
	buffer.resolved = resolved[:0]
	return resolved, nil
}

func queryStepsRequireMissing(steps []QueryStep) bool {
	for stepIndex := range steps {
		step := &steps[stepIndex]
		if step.isMissing {
			return true
		}
		for alternativeIndex := range step.alternatives {
			alternative := &step.alternatives[alternativeIndex]
			if alternative.isMissing || queryStepsRequireMissing(alternative.steps) {
				return true
			}
		}
	}
	return false
}
