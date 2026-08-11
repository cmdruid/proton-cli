// Package contentline is the line grammar iCalendar (RFC 5545) and vCard
// (RFC 6350) share.
//
// Both formats are a list of content lines shaped
// `[group.]NAME[;PARAM=value…]:value`, both fold long lines by continuing them
// with leading whitespace, and both escape the same four characters inside text
// values. Everything above that - which properties exist, what their values
// mean - belongs to the format, not here.
package contentline

import (
	"strings"
	"unicode/utf8"
)

// Param is one property parameter.
type Param struct {
	Name  string // upper-cased
	Value string
}

// Params are a property's parameters, in document order.
type Params []Param

// Get returns the first value of the named parameter, or "".
func (ps Params) Get(name string) string {
	name = strings.ToUpper(name)
	for _, p := range ps {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

// Has reports whether the named parameter is present.
func (ps Params) Has(name string) bool {
	name = strings.ToUpper(name)
	for _, p := range ps {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Line is one parsed content line. Value is the raw value: escaping is a
// property of text-typed values, so unescaping is the format's decision rather
// than the grammar's.
type Line struct {
	Group  string // vCard groups properties as "item1.EMAIL"; "" when absent
	Name   string // upper-cased
	Params Params
	Value  string
}

// Unfold splits text into logical lines, joining each continuation - a line
// beginning with a space or tab - onto the one before it.
//
// It accepts CRLF and LF alike. Proton's own cards arrive CRLF-joined, but a
// value that came from a user, a file, or another client may not.
func Unfold(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += line[1:]
			continue
		}
		out = append(out, line)
	}
	return out
}

// Parse reads one logical line. It reports false for anything without a value
// separator, which is how BEGIN-less junk and blank lines are skipped.
func Parse(raw string) (Line, bool) {
	// A colon inside a quoted parameter value is not the separator, so the scan
	// tracks quoting rather than taking the first colon.
	colon := -1
	inQuotes := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			inQuotes = !inQuotes
		case ':':
			if !inQuotes {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon <= 0 {
		return Line{}, false
	}

	name, value := raw[:colon], raw[colon+1:]
	var params Params
	if semi := indexUnquoted(name, ';'); semi >= 0 {
		params = parseParams(name[semi+1:])
		name = name[:semi]
	}
	group := ""
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		group, name = name[:dot], name[dot+1:]
	}
	return Line{Group: group, Name: strings.ToUpper(strings.TrimSpace(name)), Params: params, Value: value}, true
}

// ParseAll reads every content line in text, skipping the BEGIN and END
// delimiters: a caller working with one component does not want them, and a
// caller merging several cards must not be confused by them.
func ParseAll(text string) []Line {
	var out []Line
	for _, raw := range Unfold(text) {
		l, ok := Parse(raw)
		if !ok || l.Name == "BEGIN" || l.Name == "END" {
			continue
		}
		out = append(out, l)
	}
	return out
}

func parseParams(s string) Params {
	var out Params
	for _, part := range splitUnquoted(s, ';') {
		if part == "" {
			continue
		}
		name, value := part, ""
		if eq := strings.IndexByte(part, '='); eq >= 0 {
			name, value = part[:eq], part[eq+1:]
		}
		out = append(out, Param{
			Name:  strings.ToUpper(strings.TrimSpace(name)),
			Value: strings.Trim(value, `"`),
		})
	}
	return out
}

func indexUnquoted(s string, c byte) int {
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
		case c:
			if !inQuotes {
				return i
			}
		}
	}
	return -1
}

func splitUnquoted(s string, c byte) []string {
	var out []string
	start := 0
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
		case c:
			if !inQuotes {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// String renders the line unfolded.
func (l Line) String() string {
	var b strings.Builder
	if l.Group != "" {
		b.WriteString(l.Group)
		b.WriteByte('.')
	}
	b.WriteString(l.Name)
	for _, p := range l.Params {
		b.WriteByte(';')
		b.WriteString(p.Name)
		b.WriteByte('=')
		b.WriteString(quoteParam(p.Value))
	}
	b.WriteByte(':')
	b.WriteString(l.Value)
	return b.String()
}

// quoteParam wraps a parameter value in quotes when it holds a character that
// would otherwise end it.
func quoteParam(v string) string {
	if strings.ContainsAny(v, `;:,`) {
		return `"` + strings.ReplaceAll(v, `"`, "") + `"`
	}
	return v
}

// maxOctets is the line length RFC 5545 folds at, counting the CRLF out.
const maxOctets = 75

// Render joins lines into a document, folding any that exceed the line limit.
func Render(lines []Line) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, fold(l.String()))
	}
	return strings.Join(out, "\r\n")
}

// fold breaks a long line into continuations of at most maxOctets bytes,
// never splitting a UTF-8 sequence.
func fold(s string) string {
	if len(s) <= maxOctets {
		return s
	}
	var b strings.Builder
	limit := maxOctets
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		if cut == 0 {
			break
		}
		b.WriteString(s[:cut])
		b.WriteString("\r\n ")
		s = s[cut:]
		// A continuation starts with the space that marks it, so one octet less
		// of the value fits on every line after the first.
		limit = maxOctets - 1
	}
	b.WriteString(s)
	return b.String()
}

// EscapeText encodes a value for a text-typed property. Both RFCs name the same
// four escapes, and getting them wrong is how a description with a comma in it
// silently becomes two values.
func EscapeText(v string) string {
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case ';':
			b.WriteString(`\;`)
		case ',':
			b.WriteString(`\,`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// A CRLF in a value is one line break, so the CR is dropped and the
			// LF is what becomes \n.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// UnescapeText decodes a text-typed property value.
func UnescapeText(v string) string {
	if !strings.ContainsRune(v, '\\') {
		return v
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 >= len(v) {
			b.WriteByte(v[i])
			continue
		}
		i++
		switch v[i] {
		case 'n', 'N':
			b.WriteByte('\n')
		default:
			b.WriteByte(v[i])
		}
	}
	return b.String()
}

// SplitList splits a multi-value property value on unescaped commas, which is
// how EXDATE carries several dates and CATEGORIES several words.
func SplitList(v string) []string {
	var out []string
	start := 0
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' {
			i++
			continue
		}
		if v[i] == ',' {
			out = append(out, v[start:i])
			start = i + 1
		}
	}
	return append(out, v[start:])
}
