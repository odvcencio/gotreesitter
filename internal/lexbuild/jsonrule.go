package lexbuild

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/bits"
	"strconv"
)

// The generator reads grammar.json with serde (parse_grammar.rs: struct
// GrammarJSON, enum RuleJSON). serde rejects a rule whose "type" is unknown,
// a field of the wrong type, a missing required field, a null where no
// Option is declared, a duplicate field in a rule, and a string with an
// unpaired UTF-16 escape. The functions below apply those checks. Fields
// that serde does not know are ignored, as serde ignores them here.

// checkSurrogates rejects an unpaired \uD800-\uDFFF escape inside a JSON
// string; the generator's JSON reader refuses those, Go's decoder replaces them with U+FFFD.
func checkSurrogates(b []byte) error {
	inStr := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !inStr {
			if c == '"' {
				inStr = true
			}
			continue
		}
		switch c {
		case '"':
			inStr = false
		case '\\':
			if i+1 < len(b) && b[i+1] == 'u' && i+5 < len(b) {
				v, ok := hex4(b[i+2 : i+6])
				if ok && v >= 0xD800 && v <= 0xDBFF {
					if !(i+11 < len(b) && b[i+6] == '\\' && b[i+7] == 'u') {
						return fmt.Errorf("lone leading surrogate in a string escape")
					}
					w, ok2 := hex4(b[i+8 : i+12])
					if !ok2 || w < 0xDC00 || w > 0xDFFF {
						return fmt.Errorf("lone leading surrogate in a string escape")
					}
					i += 11
					continue
				}
				if ok && v >= 0xDC00 && v <= 0xDFFF {
					return fmt.Errorf("lone trailing surrogate in a string escape")
				}
				i += 5
				continue
			}
			i++
		}
	}
	return nil
}

func hex4(b []byte) (int, bool) {
	v := 0
	for _, c := range b {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= int(c-'A') + 10
		default:
			return 0, false
		}
	}
	return v, true
}

// decodeObject reads a JSON object into raw members. A repeated key keeps its
// first position and takes the last value (the generator's `Map<String, Value>`
// is an IndexMap under the preserve_order feature of its JSON reader); the
// repeated keys are reported, so that the caller can reject those that serde
// rejects (a recognised field of a struct, read directly).
func decodeObject(raw []byte) (keys []string, vals []json.RawMessage, dups map[string]bool, err error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, nil, nil, fmt.Errorf("expected an object")
	}
	idx := map[string]int{}
	dups = map[string]bool{}
	for dec.More() {
		before := dec.InputOffset()
		k, err := dec.Token()
		if err != nil {
			return nil, nil, nil, err
		}
		// serde decodes every key of the object as a validated string (MapKey,
		// de.rs: deserialize_str); Go's decoder replaces a lone surrogate escape
		// with U+FFFD, so the raw key (between the previous value and the colon)
		// is checked here.
		span := raw[before:dec.InputOffset()]
		if q := bytes.IndexByte(span, '"'); q >= 0 {
			if err := checkSurrogates(span[q:]); err != nil {
				return nil, nil, nil, fmt.Errorf("key: %v", err)
			}
		}
		// The value is a slice of raw, not a copy: a copy at each level of nesting
		// would hold the document once for each level (128 times at the limit).
		start := dec.InputOffset()
		var skip skipValue
		if err := dec.Decode(&skip); err != nil {
			return nil, nil, nil, err
		}
		a := start
		for a < dec.InputOffset() && (raw[a] == ':' || raw[a] == ' ' || raw[a] == '\t' || raw[a] == '\n' || raw[a] == '\r') {
			a++
		}
		v := json.RawMessage(raw[a:dec.InputOffset():dec.InputOffset()])
		ks := k.(string)
		if i, dup := idx[ks]; dup {
			dups[ks] = true
			vals[i] = v
			continue
		}
		idx[ks] = len(keys)
		keys = append(keys, ks)
		vals = append(vals, v)
	}
	if _, err := dec.Token(); err != nil {
		return nil, nil, nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, nil, fmt.Errorf("trailing data")
	}
	return keys, vals, dups, nil
}

// skipValue reads one JSON value and keeps nothing.
type skipValue struct{}

func (*skipValue) UnmarshalJSON([]byte) error { return nil }

// objView is the members of one JSON object. dupErr records how a repeated key
// was treated: as an error (serde reading a struct) or by keeping the last value
// (the generator's JSON reader reading a Value, which parse_grammar.rs does for rules).
type objView struct {
	m      map[string]json.RawMessage
	dups   map[string]bool
	dupErr bool
}

func viewObject(raw []byte, dupErr bool) (objView, error) {
	keys, vals, dups, err := decodeObject(raw)
	if err != nil {
		return objView{}, err
	}
	m := map[string]json.RawMessage{}
	for i, k := range keys {
		m[k] = vals[i]
	}
	return objView{m, dups, dupErr}, nil
}

// checkDups rejects a repeated key among the fields that serde recognises for
// the struct or enum variant being read directly. Unknown keys may repeat.
func (o objView) checkDups(known ...string) error {
	if !o.dupErr {
		return nil
	}
	for _, k := range known {
		if o.dups[k] {
			return fmt.Errorf("duplicate field `%s`", k)
		}
	}
	return nil
}

// ruleFields lists the fields serde knows for each variant of RuleJSON (the
// tag field "type" is known to all).
var ruleFields = map[string][]string{
	"ALIAS": {"content", "named", "value"}, "BLANK": {}, "STRING": {"value"},
	"PATTERN": {"value", "flags"}, "SYMBOL": {"name"}, "CHOICE": {"members"},
	"FIELD": {"name", "content"}, "SEQ": {"members"}, "REPEAT": {"content"},
	"REPEAT1": {"content"}, "PREC_DYNAMIC": {"value", "content"},
	"PREC_LEFT": {"value", "content"}, "PREC_RIGHT": {"value", "content"},
	"PREC": {"value", "content"}, "TOKEN": {"content"}, "IMMEDIATE_TOKEN": {"content"},
	"RESERVED": {"context_name", "content"}, "EOF": {},
}

// checkNumbers rejects a JSON number that the generator's JSON reader cannot represent (it
// becomes infinite as a float: "number out of range"). it checks this
// when it reads a value it keeps; it does not check numbers it skips.
func checkNumbers(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if n, ok := tok.(json.Number); ok && numberOutOfRange(string(n)) {
			return fmt.Errorf("number out of range")
		}
	}
}

// checkDepth rejects a document nested 128 levels or deeper: the JSON reader's of the generator,
// default recursion limit counts the root object as the first level, and
// reading a value deeper than the limit is an error, also for fields that are
// skipped in a rule. (the generator's JSON reader skips an unknown top-level field without counting depth.)
func checkDepth(b []byte, base int) error {
	depth, inStr := base, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			if c == '\\' {
				i++
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[', '{':
			depth++
			if depth >= 128 {
				return fmt.Errorf("recursion limit exceeded")
			}
		case ']', '}':
			depth--
		}
	}
	return nil
}

func (o objView) str(name string, required bool) (string, bool, error) {
	raw, ok := o.m[name]
	if !ok {
		if required {
			return "", false, fmt.Errorf("missing field `%s`", name)
		}
		return "", false, nil
	}
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '"' {
		if !required && string(t) == "null" {
			return "", false, nil
		}
		return "", false, fmt.Errorf("field `%s` is not a string", name)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false, fmt.Errorf("field `%s`: %v", name, err)
	}
	return s, true, nil
}

func (o objView) rule(name string) (*ruleJSON, error) {
	raw, ok := o.m[name]
	if !ok {
		return nil, fmt.Errorf("missing field `%s`", name)
	}
	return decodeRule(raw, o.dupErr)
}

// decodeRule is the serde reading of parse_grammar.rs: enum RuleJSON.
func decodeRule(raw []byte, dupErr bool) (*ruleJSON, error) {
	o, err := viewObject(raw, dupErr)
	if err != nil {
		return nil, err
	}
	typ, _, err := o.str("type", true)
	if err != nil {
		return nil, err
	}
	fields, known := ruleFields[typ]
	if !known {
		return nil, fmt.Errorf("unknown rule type %q", typ)
	}
	if err := o.checkDups(append([]string{"type"}, fields...)...); err != nil {
		return nil, err
	}
	r := &ruleJSON{Type: typ}
	needContent := func() error {
		c, err := o.rule("content")
		r.Content = c
		return err
	}
	switch typ {
	case "BLANK", "EOF":
	case "STRING":
		if _, _, err := o.str("value", true); err != nil {
			return nil, err
		}
		r.Value = o.m["value"]
	case "PATTERN":
		if _, _, err := o.str("value", true); err != nil {
			return nil, err
		}
		r.Value = o.m["value"]
		if f, has, err := o.str("flags", false); err != nil {
			return nil, err
		} else if has {
			r.Flags = &f
		}
	case "SYMBOL":
		n, _, err := o.str("name", true)
		if err != nil {
			return nil, err
		}
		r.Ident = n
	case "CHOICE", "SEQ":
		mraw, ok := o.m["members"]
		if !ok {
			return nil, fmt.Errorf("missing field `members`")
		}
		var items []json.RawMessage
		if err := json.Unmarshal(mraw, &items); err != nil || bytes.TrimSpace(mraw)[0] != '[' {
			return nil, fmt.Errorf("field `members` is not an array")
		}
		for _, it := range items {
			m, err := decodeRule(it, dupErr)
			if err != nil {
				return nil, err
			}
			r.Members = append(r.Members, *m)
		}
	case "FIELD":
		n, _, err := o.str("name", true)
		if err != nil {
			return nil, err
		}
		r.Ident = n
		if err := needContent(); err != nil {
			return nil, err
		}
	case "ALIAS":
		if err := needContent(); err != nil {
			return nil, err
		}
		braw, ok := o.m["named"]
		if !ok {
			return nil, fmt.Errorf("missing field `named`")
		}
		if err := json.Unmarshal(braw, &r.Named); err != nil || (string(bytes.TrimSpace(braw)) != "true" && string(bytes.TrimSpace(braw)) != "false") {
			return nil, fmt.Errorf("field `named` is not a boolean")
		}
		if _, _, err := o.str("value", true); err != nil {
			return nil, err
		}
		r.Value = o.m["value"]
	case "REPEAT", "REPEAT1", "TOKEN", "IMMEDIATE_TOKEN":
		if err := needContent(); err != nil {
			return nil, err
		}
	case "PREC_DYNAMIC":
		v, ok := o.m["value"]
		var n int32
		if t := bytes.TrimSpace(v); !ok || json.Unmarshal(v, &n) != nil || t[0] == '"' || string(t) == "null" || string(t) == "-0" {
			return nil, fmt.Errorf("field `value` of PREC_DYNAMIC is not an integer")
		}
		r.Value = v
		if err := needContent(); err != nil {
			return nil, err
		}
	case "PREC", "PREC_LEFT", "PREC_RIGHT":
		v, ok := o.m["value"]
		if !ok {
			return nil, fmt.Errorf("missing field `value`")
		}
		var n int32
		var s string
		if json.Unmarshal(v, &n) != nil && json.Unmarshal(v, &s) != nil {
			return nil, fmt.Errorf("field `value` of %s is neither an integer nor a string", typ)
		}
		if t := bytes.TrimSpace(v); t[0] == 'n' || string(t) == "-0" {
			return nil, fmt.Errorf("field `value` of %s is null or a float (-0)", typ)
		}
		r.Value = v
		if err := needContent(); err != nil {
			return nil, err
		}
	case "RESERVED":
		cn, _, err := o.str("context_name", true)
		if err != nil {
			return nil, err
		}
		r.ContextName = cn
		if err := needContent(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown rule type %q", typ)
	}
	return r, nil
}

// pow10 is serde_json de.rs: static POW10 (1e0 to 1e308, each the correctly
// rounded double of its literal).
var pow10 = func() [309]float64 {
	var t [309]float64
	for i := range t {
		t[i], _ = strconv.ParseFloat("1e"+strconv.Itoa(i), 64)
	}
	return t
}()

// numberOutOfRange reports whether the generator's JSON reader (the serde_json crate,
// 1.x without the float_roundtrip feature, which the generator does not
// enable) rejects the number token tok (valid JSON, as the json.Number of Go's
// decoder) with "number out of range". It ports de.rs: parse_integer,
// parse_long_integer, parse_decimal, parse_decimal_overflow, parse_exponent,
// parse_exponent_overflow and f64_from_parts (the non-float_roundtrip one): the
// significand is read as a u64 for as long as it fits, converted to f64, then
// multiplied by a power of ten of the table. Only the error decision is kept.
func numberOutOfRange(tok string) bool {
	i := 0
	if i < len(tok) && tok[i] == '-' {
		i++
	}
	isDigit := func() bool { return i < len(tok) && tok[i] >= '0' && tok[i] <= '9' }
	// fits reports whether sig*10+d fits in a u64 (the overflow! macro).
	fits := func(sig uint64, d uint64) bool {
		hi, lo := bits.Mul64(sig, 10)
		_, carry := bits.Add64(lo, d, 0)
		return hi == 0 && carry == 0
	}
	var sig uint64
	exp := int32(0)
	// parse_integer (a single leading 0 stands alone).
	if tok[i] == '0' {
		i++
	} else {
		sig = uint64(tok[i] - '0')
		i++
		for isDigit() {
			d := uint64(tok[i] - '0')
			if !fits(sig, d) {
				// parse_long_integer: the remaining digits only raise the exponent.
				for isDigit() {
					i++
					exp++
				}
				break
			}
			i++
			sig = sig*10 + d
		}
	}
	if i < len(tok) && tok[i] == '.' { // parse_decimal
		i++
		for isDigit() {
			d := uint64(tok[i] - '0')
			if !fits(sig, d) {
				// parse_decimal_overflow: ignore the rest of the digits.
				for isDigit() {
					i++
				}
				break
			}
			i++
			sig = sig*10 + d
			exp--
		}
	}
	if i < len(tok) && (tok[i] == 'e' || tok[i] == 'E') { // parse_exponent
		i++
		positive := true
		if i < len(tok) && (tok[i] == '+' || tok[i] == '-') {
			positive = tok[i] == '+'
			i++
		}
		e := int64(0)
		for isDigit() {
			d := int64(tok[i] - '0')
			if e*10+d > math.MaxInt32 {
				// parse_exponent_overflow: an error unless the significand is zero
				// or the exponent is negative.
				return sig != 0 && positive
			}
			e = e*10 + d
			i++
		}
		// saturating_add / saturating_sub on i32
		t := int64(exp)
		if positive {
			t += e
		} else {
			t -= e
		}
		if t > math.MaxInt32 {
			t = math.MaxInt32
		} else if t < math.MinInt32 {
			t = math.MinInt32
		}
		exp = int32(t)
	}
	// f64_from_parts
	f := float64(sig)
	for {
		a := int64(exp)
		if a < 0 {
			a = -a
		}
		if a < int64(len(pow10)) {
			if exp >= 0 {
				f *= pow10[a]
				return math.IsInf(f, 0)
			}
			return false
		}
		if f == 0 {
			return false
		}
		if exp >= 0 {
			return true
		}
		f /= 1e308
		exp += 308
	}
}
