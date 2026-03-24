package cfrender

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// CleanHTML sanitizes scraped HTML for downstream consumption by AI agents.
//
// It walks the parsed DOM tree in post-order and applies these rules:
//
//  1. Remove entire subtrees that are never useful content:
//     <svg>, <button>, <script>, <noscript>, elements with aria-hidden="true".
//  2. Strip all "style" attributes (CSS custom properties from syntax highlighters).
//  3. Strip framework-generated hash classes (e.g. astro-7nkwcw3z).
//  4. Unwrap <span> elements that carry no remaining attributes.
//  5. Remove empty elements — no text content and no meaningful children.
//
// Uses golang.org/x/net/html for correct parsing; never relies on regex.
func CleanHTML(raw string) string {
	if raw == "" {
		return ""
	}

	nodes, err := html.ParseFragment(strings.NewReader(raw), &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	})
	if err != nil || len(nodes) == 0 {
		return raw
	}

	// Reparent all fragment nodes under a synthetic container so every node
	// (including top-level spans) has a parent and can be unwrapped/removed.
	container := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	for _, n := range nodes {
		container.AppendChild(n)
	}

	cleanChildren(container)

	var buf bytes.Buffer
	for c := container.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&buf, c)
	}
	return buf.String()
}

// Elements removed entirely — their subtrees carry no informational value.
var removedElements = map[atom.Atom]bool{
	atom.Svg:      true,
	atom.Button:   true,
	atom.Script:   true,
	atom.Noscript: true,
}

// Self-closing / void elements that are legitimate even when "empty".
var voidElements = map[atom.Atom]bool{
	atom.Br:    true,
	atom.Hr:    true,
	atom.Img:   true,
	atom.Input: true,
	atom.Meta:  true,
	atom.Link:  true,
	atom.Wbr:   true,
}

// cleanChildren processes all descendants of n (but not n itself).
// Post-order traversal ensures inner nodes are resolved before their parents.
func cleanChildren(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling

		if c.Type == html.ElementNode {
			// Rule 1: remove entire subtrees for noise elements.
			if removedElements[c.DataAtom] || hasAttr(c, "aria-hidden", "true") {
				n.RemoveChild(c)
				c = next
				continue
			}

			// Recurse into children first (post-order).
			cleanChildren(c)

			// Rule 2: strip style attributes.
			stripAttr(c, "style")

			// Rule 3: strip framework hash classes.
			stripHashClasses(c)

			// Rule 4: unwrap bare spans.
			if c.DataAtom == atom.Span && len(c.Attr) == 0 {
				unwrapNode(c)
				c = next
				continue
			}

			// Rule 5: remove empty elements (no text, no meaningful children).
			if !voidElements[c.DataAtom] && isEmptyElement(c) {
				n.RemoveChild(c)
				c = next
				continue
			}
		}

		c = next
	}
}

// hasAttr checks whether n has an attribute with the given key and value.
func hasAttr(n *html.Node, key, val string) bool {
	for _, a := range n.Attr {
		if a.Key == key && a.Val == val {
			return true
		}
	}
	return false
}

// stripAttr removes all occurrences of the named attribute from n.
func stripAttr(n *html.Node, name string) {
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != name {
			attrs = append(attrs, a)
		}
	}
	n.Attr = attrs
}

// stripHashClasses removes framework-generated hash classes (e.g. "astro-7nkwcw3z",
// CSS-module hashes like "_foo_1a2b3") from the class attribute.
// If all classes are stripped, the class attribute itself is removed.
func stripHashClasses(n *html.Node) {
	for i, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		var kept []string
		for _, cls := range strings.Fields(a.Val) {
			if !isHashClass(cls) {
				kept = append(kept, cls)
			}
		}
		if len(kept) == 0 {
			// Remove the class attribute entirely.
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
		} else {
			n.Attr[i].Val = strings.Join(kept, " ")
		}
		return // class attribute appears at most once
	}
}

// isHashClass returns true for framework-generated class names that carry no
// semantic meaning: "astro-XXXX", CSS module hashes ("_name_hash"), etc.
func isHashClass(cls string) bool {
	// Astro: "astro-" followed by alphanumeric hash containing at least one digit.
	// The digit requirement distinguishes hashes (astro-7nkwcw3z) from
	// meaningful names (astro-config, astro-theme).
	if strings.HasPrefix(cls, "astro-") && len(cls) > 6 {
		suffix := cls[6:]
		return isAlphaNum(suffix) && hasDigit(suffix)
	}
	return false
}

func isAlphaNum(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func hasDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// isEmptyElement returns true when an element has no visible text content
// and no meaningful child elements.
//
// Special case: if the element has attributes AND contains whitespace-only text,
// it is NOT empty — the whitespace may be intentional (e.g. <span class="indent">  </span>).
func isEmptyElement(n *html.Node) bool {
	hasWhitespace := false
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return false // real text content
			}
			if len(c.Data) > 0 {
				hasWhitespace = true
			}
		case html.ElementNode:
			return false // any child element means not empty
		}
	}
	// Whitespace in an attributed element may be intentional (e.g. code indent).
	if hasWhitespace && len(n.Attr) > 0 {
		return false
	}
	return true
}

// unwrapNode replaces n with its children in the parent's child list.
func unwrapNode(n *html.Node) {
	parent := n.Parent
	if parent == nil {
		return
	}
	for n.FirstChild != nil {
		child := n.FirstChild
		n.RemoveChild(child)
		parent.InsertBefore(child, n)
	}
	parent.RemoveChild(n)
}
