// Package jsonpath selects parts of a JSON document with RFC 9535
// expressions, as $.users[?(@.age > 30)].name.
package jsonpath

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Kind uint8

const (
	Null Kind = iota
	Bool
	Number
	String
	Array
	Object
)

// Value is a JSON value that keeps the order of an object's keys and the
// text of its numbers, which map[string]any and float64 would lose: a
// filtered response should read like the response.
type Value struct {
	Kind Kind
	Bool bool
	Num  json.Number
	Str  string
	// Items are the elements of an array, or the values of an object,
	// the key of each at the same index of Keys.
	Items []*Value
	Keys  []string
}

func Parse(data []byte) (*Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decode(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("jsonpath: data after the value")
	}
	return v, nil
}

func decode(dec *json.Decoder) (*Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		v := &Value{Kind: Array}
		if t == '{' {
			v.Kind = Object
		}
		for dec.More() {
			if v.Kind == Object {
				k, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v.Keys = append(v.Keys, k.(string))
			}
			item, err := decode(dec)
			if err != nil {
				return nil, err
			}
			v.Items = append(v.Items, item)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return v, nil
	case string:
		return &Value{Kind: String, Str: t}, nil
	case json.Number:
		return &Value{Kind: Number, Num: t}, nil
	case bool:
		return &Value{Kind: Bool, Bool: t}, nil
	}
	return &Value{Kind: Null}, nil
}

// Indent returns v as JSON indented by two spaces, as json.Indent does.
func (v *Value) Indent() string {
	var b strings.Builder
	v.write(&b, "  ", 0)
	return b.String()
}

// String returns v as compact JSON.
func (v *Value) String() string {
	var b strings.Builder
	v.write(&b, "", 0)
	return b.String()
}

func (v *Value) write(b *strings.Builder, indent string, depth int) {
	switch v.Kind {
	case Null:
		b.WriteString("null")
	case Bool:
		b.WriteString(strconv.FormatBool(v.Bool))
	case Number:
		b.WriteString(v.Num.String())
	case String:
		writeString(b, v.Str)
	case Array, Object:
		open, close := byte('['), byte(']')
		if v.Kind == Object {
			open, close = '{', '}'
		}
		b.WriteByte(open)
		if len(v.Items) == 0 {
			b.WriteByte(close)
			return
		}
		newline := func(depth int) {
			if indent != "" {
				b.WriteByte('\n')
				b.WriteString(strings.Repeat(indent, depth))
			}
		}
		for i, item := range v.Items {
			if i > 0 {
				b.WriteByte(',')
			}
			newline(depth + 1)
			if v.Kind == Object {
				writeString(b, v.Keys[i])
				b.WriteByte(':')
				if indent != "" {
					b.WriteByte(' ')
				}
			}
			item.write(b, indent, depth+1)
		}
		newline(depth)
		b.WriteByte(close)
	}
}

// writeString quotes s, leaving non-ASCII and <>& as they are, unlike
// json.Marshal.
func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
