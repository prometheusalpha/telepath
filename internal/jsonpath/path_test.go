package jsonpath

import (
	"errors"
	"strings"
	"testing"
)

// store is the example of RFC 9535, section 1.5.
const store = `{ "store": {
    "book": [
      { "category": "reference", "author": "Nigel Rees", "title": "Sayings of the Century", "price": 8.95 },
      { "category": "fiction", "author": "Evelyn Waugh", "title": "Sword of Honour", "price": 12.99 },
      { "category": "fiction", "author": "Herman Melville", "title": "Moby Dick", "isbn": "0-553-21311-3", "price": 8.99 },
      { "category": "fiction", "author": "J. R. R. Tolkien", "title": "The Lord of the Rings", "isbn": "0-395-19395-8", "price": 22.99 }
    ],
    "bicycle": { "color": "red", "price": 399 }
  } }`

func selected(t *testing.T, doc, expr string) string {
	t.Helper()
	v, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Compile(expr)
	if err != nil {
		t.Fatalf("Compile(%q): %v", expr, err)
	}
	var out []string
	for _, s := range p.Select(v) {
		out = append(out, s.String())
	}
	return strings.Join(out, " ")
}

func TestSelect(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{`$.store.book[*].author`, `"Nigel Rees" "Evelyn Waugh" "Herman Melville" "J. R. R. Tolkien"`},
		{`$..author`, `"Nigel Rees" "Evelyn Waugh" "Herman Melville" "J. R. R. Tolkien"`},
		{`$.store.*.color`, `"red"`},
		{`$.store..price`, `8.95 12.99 8.99 22.99 399`},
		{`$..book[2].title`, `"Moby Dick"`},
		{`$..book[-1].title`, `"The Lord of the Rings"`},
		{`$..book[0,1].price`, `8.95 12.99`},
		{`$..book[:2].price`, `8.95 12.99`},
		{`$..book[::-2].price`, `22.99 12.99`},
		{`$..book[ 1 : 3 ].price`, `12.99 8.99`},
		{`$..book[?@.isbn].title`, `"Moby Dick" "The Lord of the Rings"`},
		{`$..book[?(!@.isbn)].price`, `8.95 12.99`},
		{`$..book[?(@.price < 10)].title`, `"Sayings of the Century" "Moby Dick"`},
		{`$..book[?(@.price >= 12.99 && @.category == 'fiction')].price`, `12.99 22.99`},
		{`$..book[?(@.price > 20 || @.author == "Nigel Rees")].price`, `8.95 22.99`},
		{`$..book[?(@.price < $.store.bicycle.price && !(@.category != "reference"))].price`, `8.95`},
		{`$..book[?(@.missing == @.alsoMissing)].price`, `8.95 12.99 8.99 22.99`},
		{`$..book[?(@.price == 8.950)].price`, `8.95`},
		{`$.store['bicycle']["color"]`, `"red"`},
		{`$.store.bicycle`, `{"color":"red","price":399}`},
		{`store.bicycle.price`, `399`},
		{`.store.bicycle.price`, `399`},
		{`[ 'store' ].bicycle.price`, `399`},
		{`$.store.book[9]`, ``},
		{`$.store.bicycle[0]`, ``},
	} {
		if got := selected(t, store, tc.expr); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.expr, got, tc.want)
		}
	}
}

func TestSelectValues(t *testing.T) {
	for _, tc := range []struct{ doc, expr, want string }{
		{`{"a":{"b":[1,2]},"c":{"b":[1,2]}}`, `$[?(@.b == $.a.b)]`, `{"b":[1,2]} {"b":[1,2]}`},
		{`[{"x":{"k":1,"j":2}},{"x":{"j":2,"k":1}},{"x":{"j":2}}]`, `$[?(@.x == $[0].x)].x`, `{"k":1,"j":2} {"j":2,"k":1}`},
		{`["b","a","c",1,true,null]`, `$[?(@ > "a")]`, `"b" "c"`},
		{`[null,false,0,""]`, `$[?(@ == null)]`, `null`},
		{`{"é":{"日本":1},"a-b":2}`, `$.é.日本`, `1`},
		{`{"a-b":2,"q'\"":3}`, `$['a-b', 'q\'"']`, `2 3`},
		{`{"\u263a":1}`, `$["\u263a"]`, `1`},
		{`{"😀":1}`, `$["\ud83d\ude00"]`, `1`},
		{`[1,{"a":2}]`, `$`, `[1,{"a":2}]`},
		{`[0,1,2,3,4,5]`, `$[1:5:2]`, `1 3`},
		{`[0,1,2,3,4,5]`, `$[5:1:-2]`, `5 3`},
		{`[0,1,2,3,4,5]`, `$[-2:]`, `4 5`},
		{`[0,1,2,3,4,5]`, `$[::0]`, ``},
		{`[0,1,2,3,4,5]`, `$[10:]`, ``},
		{`{"a":[{"b":1},{"b":2}],"c":{"b":3}}`, `$..b`, `1 2 3`},
		{`{"a":[{"b":1},{"b":2}],"c":{"b":3}}`, `$..[0]`, `{"b":1}`},
		{`{"a":{"x":1},"b":{"x":2}}`, `$[?@.x > 1]`, `{"x":2}`},
	} {
		if got := selected(t, tc.doc, tc.expr); got != tc.want {
			t.Errorf("%s on %s = %s, want %s", tc.expr, tc.doc, got, tc.want)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	for _, tc := range []struct {
		expr string
		pos  int
	}{
		{``, 0},
		{`   `, 3},
		{`$.`, 2},
		{`$.a-b`, 3},
		{`$[`, 2},
		{`$[0`, 3},
		{`$['a`, 2},
		{`$[?(@.a ==)]`, 10},
		{`$[?(@..a == 1)]`, 4},
		{`$[?(@.a == $[*])]`, 11},
		{`$[?(@.a == 1]`, 12},
		{`$[?1]`, 4},
		{`$[1:2:3:4]`, 7},
		{`$x`, 1},
		{`#`, 0},
		{`$.日本-`, 4},
	} {
		_, err := Compile(tc.expr)
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("Compile(%q) = %v, want an *Error", tc.expr, err)
			continue
		}
		if e.Pos != tc.pos {
			t.Errorf("Compile(%q) at %d (%v), want %d", tc.expr, e.Pos, e, tc.pos)
		}
	}
}

func TestSingular(t *testing.T) {
	for expr, want := range map[string]bool{
		`$`:                true,
		`$.a[0]['b']`:      true,
		`data.items[-1]`:   true,
		`$.a[*]`:           false,
		`$..a`:             false,
		`$.a[0,1]`:         false,
		`$.a[0:1]`:         false,
		`$.a[?@.b]`:        false,
		`$.a[?(@.b == 1)]`: false,
	} {
		p, err := Compile(expr)
		if err != nil {
			t.Fatal(err)
		}
		if p.Singular() != want {
			t.Errorf("Singular(%q) = %v", expr, !want)
		}
	}
}

func TestParseKeepsOrderAndNumbers(t *testing.T) {
	doc := `{"z":1,"a":{"y":12345678901234567890,"b":1.50},"m":[],"n":{},"s":"<a&b>\n\"é\"\u0001","t":true,"f":false,"u":null}`
	v, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"z":1,"a":{"y":12345678901234567890,"b":1.50},"m":[],"n":{},"s":"<a&b>\n\"é\"\u0001","t":true,"f":false,"u":null}`
	if got := v.String(); got != want {
		t.Errorf("String = %s", got)
	}
	indented := "{\n  \"z\": 1,\n  \"a\": {\n    \"y\": 12345678901234567890,\n    \"b\": 1.50\n  },\n  \"m\": [],\n  \"n\": {},\n" +
		"  \"s\": \"<a&b>\\n\\\"é\\\"\\u0001\",\n  \"t\": true,\n  \"f\": false,\n  \"u\": null\n}"
	if got := v.Indent(); got != indented {
		t.Errorf("Indent =\n%s", got)
	}
	for _, bad := range []string{``, `{`, `[1,]`, `{"a":1} 2`, `{1:2}`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) = nil error", bad)
		}
	}
}
