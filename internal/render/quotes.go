package render

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Quote-detection heuristics ported from Proton's web client:
// applications/mail/src/app/helpers/message/messageBlockquote.ts.
//
// The HTML path matches a small list of CSS-equivalent selectors and picks
// the first element where no significant content (text or image marker)
// follows. The plaintext path matches a literal forward marker or a
// canonical reply introducer ("On <date>, <name> <email@addr> wrote:").
//
// Both are conservative: when the body shape doesn't match any pattern,
// the function returns the input unchanged. Some non-standard quoting
// styles will be preserved in the output.

const (
	originalMessageMarker = "------- Original Message -------"
	forwardedMarker       = "------- Forwarded Message -------"
)

// htmlQuoteSelector describes one entry in the canonical quote-selector
// list. A field set to its zero value imposes no constraint.
type htmlQuoteSelector struct {
	tag       string // element tag (lower-case); empty = any
	class     string // required class token (one of element's whitespace-separated classes)
	id        string // required id attribute value
	attrName  string // required attribute name
	attrValue string // required attribute value when attrName is set; empty = presence only
}

func (s htmlQuoteSelector) matches(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if s.tag != "" && n.Data != s.tag {
		return false
	}
	var classes, id string
	attrs := make(map[string]string, len(n.Attr))
	for _, a := range n.Attr {
		switch a.Key {
		case "class":
			classes = a.Val
		case "id":
			id = a.Val
		}
		attrs[a.Key] = a.Val
	}
	if s.class != "" && !classListContains(classes, s.class) {
		return false
	}
	if s.id != "" && id != s.id {
		return false
	}
	if s.attrName != "" {
		v, ok := attrs[s.attrName]
		if !ok {
			return false
		}
		if s.attrValue != "" && v != s.attrValue {
			return false
		}
	}
	return true
}

// quoteSelectors is the canonical list, mirroring Proton's BLOCKQUOTE_SELECTORS.
var quoteSelectors = []htmlQuoteSelector{
	{class: "protonmail_quote"},                                 // Proton Mail
	{class: "gmail_quote"},                                      // Gmail (div or blockquote)
	{tag: "div", class: "gmail_extra"},                          // Gmail
	{tag: "div", class: "yahoo_quoted"},                         // Yahoo Mail
	{tag: "blockquote", class: "iosymail"},                      // Yahoo iOS
	{class: "tutanota_quote"},                                   // Tutanota
	{class: "zmail_extra"},                                      // Zoho
	{class: "skiff_quote"},                                      // Skiff
	{tag: "blockquote", attrName: "data-skiff-mail"},            // Skiff
	{id: "divRplyFwdMsg"},                                       // Outlook
	{tag: "div", id: "mail-editor-reference-message-container"}, // Outlook
	{tag: "hr", id: "replySplit"},                               //
	{class: "moz-cite-prefix"},                                  // Thunderbird
	{tag: "div", id: "isForwardContent"},                        //
	{tag: "blockquote", id: "isReplyContent"},                   //
	{tag: "div", id: "mailcontent"},                             //
	{tag: "div", id: "origbody"},                                //
	{tag: "div", id: "reply139content"},                         //
	{tag: "blockquote", id: "oriMsgHtmlSeperator"},              //
	{tag: "blockquote", attrName: "type", attrValue: "cite"},    // generic
	{attrName: "name", attrValue: "quote"},                      // gmx
}

// StripHTMLQuotes returns html with the canonical quote subtree removed.
// Returns the input unchanged when no qualifying quote is found or the
// input fails to parse.
//
// "Qualifying" means: the element matches a known quote selector AND no
// significant content (non-whitespace text, or the proton-image-anchor
// marker) follows it in document order. The first such element is
// removed. A subsequent fallback matches the literal phrase
// "------- Original Message -------" anywhere in a text node.
func StripHTMLQuotes(s string) string {
	if s == "" {
		return s
	}
	bodyCtx := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(s), bodyCtx)
	if err != nil {
		return s
	}
	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	candidate := findSelectorQuote(root)
	if candidate == nil {
		candidate = findOriginalMessageQuote(root)
	}
	if candidate == nil || candidate.Parent == nil {
		return s
	}
	candidate.Parent.RemoveChild(candidate)

	var b strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return s
		}
	}
	return b.String()
}

func findSelectorQuote(root *html.Node) *html.Node {
	var candidates []*html.Node
	walkNodes(root, func(n *html.Node) {
		for _, sel := range quoteSelectors {
			if sel.matches(n) {
				candidates = append(candidates, n)
				return
			}
		}
	})
	for _, c := range candidates {
		if !hasSignificantContentAfter(c, root) {
			return c
		}
	}
	return nil
}

func findOriginalMessageQuote(root *html.Node) *html.Node {
	var hit *html.Node
	walkNodes(root, func(n *html.Node) {
		if hit != nil {
			return
		}
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) == originalMessageMarker {
			parent := n.Parent
			if parent != nil && !hasSignificantContentAfter(parent, root) {
				hit = parent
			}
		}
	})
	return hit
}

// walkNodes invokes fn on every node in the tree rooted at n, in document order.
func walkNodes(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNodes(c, fn)
	}
}

// hasSignificantContentAfter reports whether any node positioned after
// target in document order (excluding target's descendants) is either a
// non-whitespace text node or carries the proton-image-anchor class.
func hasSignificantContentAfter(target, root *html.Node) bool {
	var ordered []*html.Node
	walkNodes(root, func(n *html.Node) { ordered = append(ordered, n) })
	last := lastDescendant(target)
	startIdx := -1
	for i, n := range ordered {
		if n == last {
			startIdx = i + 1
			break
		}
	}
	if startIdx < 0 {
		return false
	}
	for _, n := range ordered[startIdx:] {
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
			return true
		}
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "class" && classListContains(a.Val, "proton-image-anchor") {
					return true
				}
			}
		}
	}
	return false
}

func lastDescendant(n *html.Node) *html.Node {
	for n.LastChild != nil {
		n = n.LastChild
	}
	return n
}

func classListContains(classAttr, want string) bool {
	for _, c := range strings.Fields(classAttr) {
		if c == want {
			return true
		}
	}
	return false
}

// Plaintext quote detection.

var (
	emailInLineRegex     = regexp.MustCompile(`<[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+>`)
	replyIntroducerRegex = regexp.MustCompile(`(?m)^[^\n]*<[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+>[^\n]*:\s*\n\s*\n>`)
)

// StripPlaintextQuotes returns text with the first canonical forward or
// reply marker (and everything after it) removed. Returns text unchanged
// when no canonical marker is found.
//
// Forward marker: literal "------- Forwarded Message -------".
// Reply marker:   line ending ":" containing "<email@addr>", followed
//
//	(after a blank line) by a line beginning ">".
func StripPlaintextQuotes(s string) string {
	if s == "" {
		return s
	}
	if i := strings.Index(s, forwardedMarker); i >= 0 {
		return s[:i]
	}
	// Cheap pre-check (mirrors Proton's web client): a line ending with
	// ':' that contains an email in angle brackets.
	cheap := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasSuffix(line, ":") && emailInLineRegex.MatchString(line) {
			cheap = true
			break
		}
	}
	if !cheap {
		return s
	}
	if loc := replyIntroducerRegex.FindStringIndex(s); loc != nil {
		return s[:loc[0]]
	}
	return s
}

// MessagePreview returns the first non-empty line of body after
// quote-stripping for the appropriate format. mimeType picks the strip
// variant: HTML bodies are stripped of the canonical quote subtree and
// then converted to text via HTMLToText; plaintext bodies are stripped
// directly. Returns "" when the body is empty after stripping.
func MessagePreview(body, mimeType string) string {
	var text string
	if IsHTML(mimeType) {
		text = HTMLToText(StripHTMLQuotes(body))
	} else {
		text = StripPlaintextQuotes(body)
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
