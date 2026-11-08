package jsonpath

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Error is an expression that doesn't compile, Pos in runes, as inputs
// count.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s at character %d", e.Msg, e.Pos+1) }

type Path struct {
	segs []segment
}

type segment struct {
	desc bool // ..: the selectors apply to the node and all below it
	sels []selector
}

type selKind uint8

const (
	selName selKind = iota
	selWild
	selIndex
	selSlice
	selFilter
)

type selector struct {
	kind  selKind
	name  string
	index int
	// slice bounds, nil where left out
	start, end, step *int
	filter           expr
}

// Compile reads an expression. The leading $ may be left out, as
// data.items or [0], which is how people tend to type into a filter box.
func Compile(expr string) (*Path, error) {
	p := &parser{s: expr}
	p.space()
	if p.pos == len(p.s) {
		return nil, p.errorf("empty expression")
	}
	var segs []segment
	if p.eat("$") {
		// the usual case
	} else if r, _ := utf8.DecodeRuneInString(p.s[p.pos:]); nameFirst(r) {
		name, _ := p.name()
		segs = append(segs, segment{sels: []selector{{kind: selName, name: name}}})
	} else if p.peek() != '.' && p.peek() != '[' {
		return nil, p.errorf("an expression starts with $")
	}
	rest, err := p.segments()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.pos < len(p.s) {
		return nil, p.unexpected()
	}
	return &Path{segs: append(segs, rest...)}, nil
}

// Singular tells whether the expression points at one value at most: it
// names keys and indexes only.
func (p *Path) Singular() bool { return singular(p.segs) }

func singular(segs []segment) bool {
	for _, s := range segs {
		if s.desc || len(s.sels) != 1 || (s.sels[0].kind != selName && s.sels[0].kind != selIndex) {
			return false
		}
	}
	return true
}

// Select returns the values the expression points at, in document order.
func (p *Path) Select(root *Value) []*Value { return selectSegs(p.segs, root, root) }

func selectSegs(segs []segment, node, root *Value) []*Value {
	nodes := []*Value{node}
	for _, s := range segs {
		var next []*Value
		for _, n := range nodes {
			targets := []*Value{n}
			if s.desc {
				targets = descendants(n, targets[:0])
			}
			for _, t := range targets {
				for i := range s.sels {
					next = s.sels[i].apply(t, root, next)
				}
			}
		}
		nodes = next
	}
	return nodes
}

func descendants(v *Value, out []*Value) []*Value {
	out = append(out, v)
	for _, item := range v.Items {
		out = descendants(item, out)
	}
	return out
}

func (s *selector) apply(v, root *Value, out []*Value) []*Value {
	switch s.kind {
	case selName:
		if v.Kind == Object {
			for i, k := range v.Keys {
				if k == s.name {
					out = append(out, v.Items[i])
				}
			}
		}
	case selWild:
		if v.Kind == Array || v.Kind == Object {
			out = append(out, v.Items...)
		}
	case selIndex:
		if v.Kind == Array {
			i := s.index
			if i < 0 {
				i += len(v.Items)
			}
			if i >= 0 && i < len(v.Items) {
				out = append(out, v.Items[i])
			}
		}
	case selSlice:
		if v.Kind == Array {
			out = s.slice(v.Items, out)
		}
	case selFilter:
		if v.Kind == Array || v.Kind == Object {
			for _, item := range v.Items {
				if s.filter.test(item, root) {
					out = append(out, item)
				}
			}
		}
	}
	return out
}

// slice follows RFC 9535 section 2.3.4.2.2.
func (s *selector) slice(items, out []*Value) []*Value {
	n := len(items)
	step := 1
	if s.step != nil {
		step = *s.step
	}
	if step == 0 {
		return out
	}
	norm := func(i int) int {
		if i < 0 {
			return n + i
		}
		return i
	}
	clamp := func(i, lo, hi int) int { return min(max(i, lo), hi) }
	if step > 0 {
		start, end := 0, n
		if s.start != nil {
			start = *s.start
		}
		if s.end != nil {
			end = *s.end
		}
		for i := clamp(norm(start), 0, n); i < clamp(norm(end), 0, n); i += step {
			out = append(out, items[i])
		}
		return out
	}
	start, end := n-1, -n-1
	if s.start != nil {
		start = *s.start
	}
	if s.end != nil {
		end = *s.end
	}
	for i := clamp(norm(start), -1, n-1); i > clamp(norm(end), -1, n-1); i += step {
		out = append(out, items[i])
	}
	return out
}

// The filter of ?(…): logical expressions over comparisons and tests.
type expr interface {
	test(cur, root *Value) bool
}

type orExpr []expr
type andExpr []expr
type notExpr struct{ e expr }

// existExpr is a query standing alone, true when it selects anything.
type existExpr struct{ q *query }

type cmpExpr struct {
	op   string
	l, r operand
}

// operand is a literal or a singular query.
type operand struct {
	lit *Value
	q   *query
}

type query struct {
	abs  bool // $ rather than @
	segs []segment
}

func (q *query) eval(cur, root *Value) []*Value {
	if q.abs {
		return selectSegs(q.segs, root, root)
	}
	return selectSegs(q.segs, cur, root)
}

func (e orExpr) test(cur, root *Value) bool {
	for _, x := range e {
		if x.test(cur, root) {
			return true
		}
	}
	return false
}

func (e andExpr) test(cur, root *Value) bool {
	for _, x := range e {
		if !x.test(cur, root) {
			return false
		}
	}
	return true
}

func (e notExpr) test(cur, root *Value) bool   { return !e.e.test(cur, root) }
func (e existExpr) test(cur, root *Value) bool { return len(e.q.eval(cur, root)) > 0 }

// value is nil for a query that selects nothing.
func (o operand) value(cur, root *Value) *Value {
	if o.lit != nil {
		return o.lit
	}
	if got := o.q.eval(cur, root); len(got) == 1 {
		return got[0]
	}
	return nil
}

func (e cmpExpr) test(cur, root *Value) bool {
	l, r := e.l.value(cur, root), e.r.value(cur, root)
	switch e.op {
	case "==":
		return equal(l, r)
	case "!=":
		return !equal(l, r)
	case "<":
		return less(l, r)
	case ">":
		return less(r, l)
	case "<=":
		return less(l, r) || equal(l, r)
	}
	return less(r, l) || equal(l, r)
}

// equal treats two empty queries as equal, as the RFC does.
func equal(a, b *Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Null:
		return true
	case Bool:
		return a.Bool == b.Bool
	case Number:
		x, _ := a.Num.Float64()
		y, _ := b.Num.Float64()
		return x == y
	case String:
		return a.Str == b.Str
	case Array:
		if len(a.Items) != len(b.Items) {
			return false
		}
		for i := range a.Items {
			if !equal(a.Items[i], b.Items[i]) {
				return false
			}
		}
		return true
	}
	if len(a.Items) != len(b.Items) {
		return false
	}
	for i, k := range a.Keys {
		j := index(b.Keys, k)
		if j < 0 || !equal(a.Items[i], b.Items[j]) {
			return false
		}
	}
	return true
}

func index(keys []string, k string) int {
	for i, x := range keys {
		if x == k {
			return i
		}
	}
	return -1
}

// less orders numbers and strings (by code point, as Go compares UTF-8);
// anything else is not ordered.
func less(a, b *Value) bool {
	if a == nil || b == nil || a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Number:
		x, _ := a.Num.Float64()
		y, _ := b.Num.Float64()
		return x < y
	case String:
		return a.Str < b.Str
	}
	return false
}

type parser struct {
	s   string
	pos int
}

func (p *parser) errorf(format string, args ...any) error {
	return &Error{Pos: utf8.RuneCountInString(p.s[:p.pos]), Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) unexpected() error {
	if p.pos >= len(p.s) {
		return p.errorf("unexpected end")
	}
	r, _ := utf8.DecodeRuneInString(p.s[p.pos:])
	return p.errorf("unexpected %q", r)
}

func (p *parser) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *parser) eat(tok string) bool {
	if strings.HasPrefix(p.s[p.pos:], tok) {
		p.pos += len(tok)
		return true
	}
	return false
}

func (p *parser) space() {
	for p.pos < len(p.s) && strings.IndexByte(" \t\n\r", p.s[p.pos]) >= 0 {
		p.pos++
	}
}

// segments reads segments for as long as they come; space may sit between
// them, so a filter can go on with an operator after its query.
func (p *parser) segments() ([]segment, error) {
	var segs []segment
	for {
		save := p.pos
		p.space()
		var s segment
		switch {
		case p.eat(".."):
			s.desc = true
			if p.peek() == '[' {
				sels, err := p.bracket()
				if err != nil {
					return nil, err
				}
				s.sels = sels
			} else {
				sel, err := p.dotted()
				if err != nil {
					return nil, err
				}
				s.sels = []selector{sel}
			}
		case p.eat("."):
			sel, err := p.dotted()
			if err != nil {
				return nil, err
			}
			s.sels = []selector{sel}
		case p.peek() == '[':
			sels, err := p.bracket()
			if err != nil {
				return nil, err
			}
			s.sels = sels
		default:
			p.pos = save
			return segs, nil
		}
		segs = append(segs, s)
	}
}

// dotted reads what follows a dot: * or a name.
func (p *parser) dotted() (selector, error) {
	if p.eat("*") {
		return selector{kind: selWild}, nil
	}
	name, ok := p.name()
	if !ok {
		return selector{}, p.errorf("expected a name after the dot; use ['…'] for other keys")
	}
	return selector{kind: selName, name: name}, nil
}

func nameFirst(r rune) bool {
	return r == '_' || r >= 0x80 && r != utf8.RuneError || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func (p *parser) name() (string, bool) {
	start := p.pos
	for p.pos < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.pos:])
		if !nameFirst(r) && (p.pos == start || r < '0' || r > '9') {
			break
		}
		p.pos += size
	}
	return p.s[start:p.pos], p.pos > start
}

func (p *parser) bracket() ([]selector, error) {
	p.pos++ // [
	var sels []selector
	for {
		p.space()
		sel, err := p.selector()
		if err != nil {
			return nil, err
		}
		sels = append(sels, sel)
		p.space()
		if p.eat("]") {
			return sels, nil
		}
		if !p.eat(",") {
			return nil, p.unexpected()
		}
	}
}

func (p *parser) selector() (selector, error) {
	switch c := p.peek(); {
	case c == '\'' || c == '"':
		s, err := p.str()
		return selector{kind: selName, name: s}, err
	case p.eat("*"):
		return selector{kind: selWild}, nil
	case p.eat("?"):
		e, err := p.or()
		return selector{kind: selFilter, filter: e}, err
	case c == '-' || c >= '0' && c <= '9' || c == ':':
		return p.indexOrSlice()
	}
	return selector{}, p.unexpected()
}

func (p *parser) indexOrSlice() (selector, error) {
	var bounds [3]*int
	for i := range bounds {
		p.space()
		if c := p.peek(); c == '-' || c >= '0' && c <= '9' {
			n, err := p.int()
			if err != nil {
				return selector{}, err
			}
			bounds[i] = &n
		}
		p.space()
		if i == 0 && bounds[0] != nil && p.peek() != ':' {
			return selector{kind: selIndex, index: *bounds[0]}, nil
		}
		if i == 2 || !p.eat(":") {
			break
		}
	}
	return selector{kind: selSlice, start: bounds[0], end: bounds[1], step: bounds[2]}, nil
}

func (p *parser) int() (int, error) {
	start := p.pos
	p.eat("-")
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
	}
	n, err := strconv.Atoi(p.s[start:p.pos])
	if err != nil {
		p.pos = start
		return 0, p.errorf("bad number")
	}
	return n, nil
}

// str reads a quoted string, in ' or ", with JSON's escapes.
func (p *parser) str() (string, error) {
	q := p.s[p.pos]
	start := p.pos
	p.pos++
	var b strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c == q:
			p.pos++
			return b.String(), nil
		case c == '\\':
			p.pos++
			r, err := p.escape()
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
		default:
			r, size := utf8.DecodeRuneInString(p.s[p.pos:])
			b.WriteRune(r)
			p.pos += size
		}
	}
	p.pos = start
	return "", p.errorf("unclosed string")
}

func (p *parser) escape() (rune, error) {
	c := p.peek()
	p.pos++
	switch c {
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case '/', '\\', '\'', '"':
		return rune(c), nil
	case 'u':
		r, err := p.hex4()
		if err != nil {
			return 0, err
		}
		if utf16.IsSurrogate(r) && p.eat(`\u`) {
			lo, err := p.hex4()
			if err != nil {
				return 0, err
			}
			r = utf16.DecodeRune(r, lo)
		}
		return r, nil
	}
	p.pos--
	return 0, p.errorf("bad escape")
}

func (p *parser) hex4() (rune, error) {
	if p.pos+4 > len(p.s) {
		return 0, p.errorf("bad \\u escape")
	}
	n, err := strconv.ParseUint(p.s[p.pos:p.pos+4], 16, 32)
	if err != nil {
		return 0, p.errorf("bad \\u escape")
	}
	p.pos += 4
	return rune(n), nil
}

func (p *parser) or() (expr, error) {
	var list orExpr
	for {
		e, err := p.and()
		if err != nil {
			return nil, err
		}
		list = append(list, e)
		p.space()
		if !p.eat("||") {
			break
		}
	}
	if len(list) == 1 {
		return list[0], nil
	}
	return list, nil
}

func (p *parser) and() (expr, error) {
	var list andExpr
	for {
		e, err := p.basic()
		if err != nil {
			return nil, err
		}
		list = append(list, e)
		p.space()
		if !p.eat("&&") {
			break
		}
	}
	if len(list) == 1 {
		return list[0], nil
	}
	return list, nil
}

func (p *parser) basic() (expr, error) {
	p.space()
	if p.eat("!") {
		p.space()
		e, err := p.paren()
		if err != nil {
			return nil, err
		}
		if e == nil {
			q, err := p.query()
			if err != nil {
				return nil, err
			}
			e = existExpr{q}
		}
		return notExpr{e}, nil
	}
	if e, err := p.paren(); e != nil || err != nil {
		return e, err
	}
	start := p.pos
	l, err := p.operand()
	if err != nil {
		return nil, err
	}
	p.space()
	op := p.op()
	if op == "" {
		if l.q == nil {
			return nil, p.unexpected()
		}
		return existExpr{l.q}, nil
	}
	if l.q != nil && !singular(l.q.segs) {
		p.pos = start
		return nil, p.errorf("a comparison needs a query of one value")
	}
	p.space()
	start = p.pos
	r, err := p.operand()
	if err != nil {
		return nil, err
	}
	if r.q != nil && !singular(r.q.segs) {
		p.pos = start
		return nil, p.errorf("a comparison needs a query of one value")
	}
	return cmpExpr{op: op, l: l, r: r}, nil
}

// paren reads a parenthesized expression, nil when there's none.
func (p *parser) paren() (expr, error) {
	if !p.eat("(") {
		return nil, nil
	}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	p.space()
	if !p.eat(")") {
		return nil, p.unexpected()
	}
	return e, nil
}

func (p *parser) op() string {
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if p.eat(op) {
			return op
		}
	}
	return ""
}

func (p *parser) query() (*query, error) {
	q := &query{}
	switch {
	case p.eat("$"):
		q.abs = true
	case p.eat("@"):
	default:
		return nil, p.unexpected()
	}
	segs, err := p.segments()
	q.segs = segs
	return q, err
}

func (p *parser) operand() (operand, error) {
	switch c := p.peek(); {
	case c == '$' || c == '@':
		q, err := p.query()
		return operand{q: q}, err
	case c == '\'' || c == '"':
		s, err := p.str()
		return operand{lit: &Value{Kind: String, Str: s}}, err
	case c == '-' || c >= '0' && c <= '9':
		return p.number()
	case p.eat("true"):
		return operand{lit: &Value{Kind: Bool, Bool: true}}, nil
	case p.eat("false"):
		return operand{lit: &Value{Kind: Bool}}, nil
	case p.eat("null"):
		return operand{lit: &Value{Kind: Null}}, nil
	}
	return operand{}, p.unexpected()
}

func (p *parser) number() (operand, error) {
	start := p.pos
	for p.pos < len(p.s) && strings.IndexByte("-+.eE0123456789", p.s[p.pos]) >= 0 {
		p.pos++
	}
	text := p.s[start:p.pos]
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		p.pos = start
		return operand{}, p.errorf("bad number")
	}
	return operand{lit: &Value{Kind: Number, Num: json.Number(text)}}, nil
}
