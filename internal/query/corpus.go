package query

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Unclassified is the class that the harness gives a case it adds to the
// corpus from a source list. Validate rejects it: a person assigns the class.
const Unclassified = "unclassified"

// Case is one line of corpus 1: a query run on an input in one language, and
// pinned C's records for it.
//
// IDs holds every name the case has in its sources, each written
// "source:name" (for example "review:J04" or "guard:K01_order_across_roots");
// cases that were duplicates by language, query and input are one case with
// several ids. Class groups cases by the C mechanism they exercise.
//
// Usable reports whether the Go tree for Input equals pinned C's tree, as
// the harness's structural comparison decides; when it is false the matches
// of the two engines are not comparable. Tree is the digest of pinned C's
// tree (TreeHasher).
//
// L1 to L4 hold pinned C's records of each layer in text form, in order. The
// first L2 record is the verdict: accept, or reject with the error kind and
// offset. A query that C rejects has no L1, L3 or L4 records. L5 is not
// recorded yet.
type Case struct {
	IDs      []string `json:"ids"`
	Language string   `json:"language"`
	Class    string   `json:"class"`
	Query    string   `json:"query"`
	Input    string   `json:"input"`
	Usable   bool     `json:"usable"`
	Tree     string   `json:"tree"`
	L1       []string `json:"l1"`
	L2       []string `json:"l2"`
	L3       []string `json:"l3"`
	L4       []string `json:"l4"`
}

// ID returns the case's first id, which names it in test output.
func (c *Case) ID() string {
	if len(c.IDs) == 0 {
		return ""
	}
	return c.IDs[0]
}

// Key is what makes two cases duplicates: the same language, query and input.
type Key struct {
	Language string
	Query    string
	Input    string
}

// Key returns the case's duplicate key.
func (c *Case) Key() Key { return Key{Language: c.Language, Query: c.Query, Input: c.Input} }

// Verdict returns the case's first L2 record: whether pinned C accepts the
// query. It is empty when the case has no records yet.
func (c *Case) Verdict() string {
	if len(c.L2) == 0 {
		return ""
	}
	return c.L2[0]
}

// Accepted reports whether pinned C accepts the case's query.
func (c *Case) Accepted() bool { return c.Verdict() == Verdict{Accepted: true}.String() }

// IDSource returns the source part of an id ("review" for "review:J04").
func IDSource(id string) string {
	source, _, _ := strings.Cut(id, ":")
	return source
}

// ReadCorpus reads a corpus file: one JSON case per line. It checks that
// every case has ids, a language, a query and a class, and that no id and no
// duplicate key appears twice. It does not reject Unclassified cases.
func ReadCorpus(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<24)
	line := 0
	for scanner.Scan() {
		line++
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(text))
		decoder.DisallowUnknownFields()
		var c Case
		if err := decoder.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		cases = append(cases, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := checkCases(cases, false); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cases, nil
}

// Validate checks the corpus as ReadCorpus does and also rejects cases whose
// class is Unclassified.
func Validate(cases []Case) error { return checkCases(cases, true) }

func checkCases(cases []Case, requireClass bool) error {
	ids := make(map[string]bool)
	keys := make(map[Key]string)
	var errs []error
	for i := range cases {
		c := &cases[i]
		if len(c.IDs) == 0 {
			errs = append(errs, fmt.Errorf("case %d has no id", i))
			continue
		}
		for _, id := range c.IDs {
			if source, name, ok := strings.Cut(id, ":"); !ok || source == "" || name == "" {
				errs = append(errs, fmt.Errorf("case %s: id %q is not source:name", c.ID(), id))
			}
			if ids[id] {
				errs = append(errs, fmt.Errorf("id %q appears twice", id))
			}
			ids[id] = true
		}
		if c.Language == "" || c.Query == "" {
			errs = append(errs, fmt.Errorf("case %s has no language or query", c.ID()))
		}
		if c.Class == "" || (requireClass && c.Class == Unclassified) {
			errs = append(errs, fmt.Errorf("case %s has no class", c.ID()))
		}
		if first, ok := keys[c.Key()]; ok {
			errs = append(errs, fmt.Errorf("cases %s and %s have the same language, query and input", first, c.ID()))
		}
		keys[c.Key()] = c.ID()
	}
	return errors.Join(errs...)
}

// WriteCorpus writes the cases to path, one JSON object per line, in the
// given order. Empty record lists are written as [] so that "no matches" is
// explicit.
func WriteCorpus(path string, cases []Case) error {
	if err := checkCases(cases, false); err != nil {
		return err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, c := range cases {
		for _, records := range []*[]string{&c.L1, &c.L2, &c.L3, &c.L4} {
			if *records == nil {
				*records = []string{}
			}
		}
		if err := encoder.Encode(c); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
