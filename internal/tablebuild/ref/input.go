package ref

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// GrammarRecords returns the L0 records of a grammar.json: one record per
// rule in file order, one per element of each other top-level list, and one
// per other top-level value. Values are written as compact JSON with the
// object keys in file order. "$schema" names the file format and is not read.
func GrammarRecords(grammarJSON []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(grammarJSON))
	dec.UseNumber()
	if err := expectDelim(dec, '{'); err != nil {
		return nil, fmt.Errorf("grammar.json: %w", err)
	}
	var out []string
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return nil, fmt.Errorf("grammar.json: %w", err)
		}
		switch {
		case key == "rules":
			if err := expectDelim(dec, '{'); err != nil {
				return nil, fmt.Errorf("grammar.json rules: %w", err)
			}
			for dec.More() {
				name, err := objectKey(dec)
				if err != nil {
					return nil, fmt.Errorf("grammar.json rules: %w", err)
				}
				var b strings.Builder
				if err := canonical(dec, &b); err != nil {
					return nil, fmt.Errorf("grammar.json rule %s: %w", name, err)
				}
				out = append(out, line("L0", "rule", strconv.Quote(name), b.String()))
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
		default:
			tok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("grammar.json %s: %w", key, err)
			}
			if d, ok := tok.(json.Delim); ok && d == '[' {
				for i := 0; dec.More(); i++ {
					var b strings.Builder
					if err := canonical(dec, &b); err != nil {
						return nil, fmt.Errorf("grammar.json %s: %w", key, err)
					}
					out = append(out, line("L0", key, itoa(i), b.String()))
				}
				if _, err := dec.Token(); err != nil {
					return nil, err
				}
				continue
			}
			var b strings.Builder
			if err := canonicalFrom(dec, tok, &b); err != nil {
				return nil, fmt.Errorf("grammar.json %s: %w", key, err)
			}
			if key != "$schema" {
				out = append(out, line("L0", key, b.String()))
			}
		}
	}
	return out, nil
}

// NodeTypeRecords returns one L1 record per entry of a node-types.json, in
// file order, as compact JSON with the keys in file order.
func NodeTypeRecords(nodeTypesJSON []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(nodeTypesJSON))
	dec.UseNumber()
	if err := expectDelim(dec, '['); err != nil {
		return nil, fmt.Errorf("node-types.json: %w", err)
	}
	var out []string
	for dec.More() {
		var b strings.Builder
		if err := canonical(dec, &b); err != nil {
			return nil, fmt.Errorf("node-types.json: %w", err)
		}
		out = append(out, line("L1", "node-type", b.String()))
	}
	return out, nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != want {
		return fmt.Errorf("want %q, got %v", want, tok)
	}
	return nil
}

func objectKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	k, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("want an object key, got %v", tok)
	}
	return k, nil
}

// canonical copies one JSON value from dec to w compactly, keeping the order
// of object keys and the text of numbers. Strings are written with
// strconv.Quote.
func canonical(dec *json.Decoder, w io.StringWriter) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	return canonicalFrom(dec, tok, w)
}

// canonicalFrom is canonical for a value whose first token was read.
func canonicalFrom(dec *json.Decoder, tok json.Token, w io.StringWriter) error {
	switch t := tok.(type) {
	case json.Delim:
		close := "]"
		if t == '{' {
			close = "}"
		}
		w.WriteString(t.String())
		for first := true; dec.More(); first = false {
			if !first {
				w.WriteString(",")
			}
			if t == '{' {
				k, err := objectKey(dec)
				if err != nil {
					return err
				}
				w.WriteString(strconv.Quote(k) + ":")
			}
			if err := canonical(dec, w); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		w.WriteString(close)
	case string:
		w.WriteString(strconv.Quote(t))
	case json.Number:
		w.WriteString(t.String())
	case bool:
		w.WriteString(strconv.FormatBool(t))
	case nil:
		w.WriteString("null")
	default:
		return fmt.Errorf("unexpected token %v", tok)
	}
	return nil
}
