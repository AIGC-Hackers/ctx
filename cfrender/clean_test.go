package cfrender

import "testing"

// BDD-style specs for CleanHTML.
//
// CleanHTML is a critical path for agent-consumed content: silent corruption
// (swallowed text, reordered tokens, mangled attributes) can mislead
// downstream models with no easy way to detect the problem.
//
// The specs are grouped by behavior rather than implementation detail.

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Style attribute removal
// CleanHTML MUST remove all inline style attributes regardless of element type,
// attribute quoting, or value complexity, without altering any other attribute
// or the element's text content.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_RemovesStyleFromSpan(t *testing.T) {
	in := `<span style="--0:#C792EA;--1:#8844AE">import</span>`
	want := `import` // bare span also gets unwrapped
	assertClean(t, in, want)
}

func TestCleanHTML_RemovesStyleFromDiv(t *testing.T) {
	in := `<div style="color:red">hello</div>`
	want := `<div>hello</div>`
	assertClean(t, in, want)
}

func TestCleanHTML_RemovesStyleButKeepsOtherAttrs(t *testing.T) {
	in := `<span class="token keyword" style="--0:#C792EA">const</span>`
	want := `<span class="token keyword">const</span>`
	assertClean(t, in, want)
}

func TestCleanHTML_RemovesStyleWithSingleQuotes(t *testing.T) {
	in := `<span style='color:red'>x</span>`
	want := `x`
	assertClean(t, in, want)
}

func TestCleanHTML_RemovesStyleWithEmptyValue(t *testing.T) {
	in := `<span style="">x</span>`
	want := `x`
	assertClean(t, in, want)
}

func TestCleanHTML_RemovesMultipleStyleAttrs(t *testing.T) {
	in := `<span style="a:1">x</span><span style="b:2">y</span>`
	want := `xy`
	assertClean(t, in, want)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Bare span unwrapping
// A <span> with ZERO attributes after style removal MUST be replaced by its
// children. This applies recursively: nested bare spans all collapse.
// A <span> that retains any attribute (class, id, data-*, etc.) MUST be kept.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_UnwrapsBareSingleSpan(t *testing.T) {
	assertClean(t, `<span>text</span>`, `text`)
}

func TestCleanHTML_UnwrapsNestedBareSpans(t *testing.T) {
	assertClean(t,
		`<span style="a"><span style="b">inner</span></span>`,
		`inner`)
}

func TestCleanHTML_PreservesSpanWithClass(t *testing.T) {
	in := `<span class="indent">  </span>`
	assertClean(t, in, in)
}

func TestCleanHTML_PreservesSpanWithID(t *testing.T) {
	in := `<span id="marker">x</span>`
	assertClean(t, in, in)
}

func TestCleanHTML_PreservesSpanWithDataAttr(t *testing.T) {
	in := `<span data-line="5">x</span>`
	assertClean(t, in, in)
}

func TestCleanHTML_UnwrapsOnlyStyleSpanKeepsClassSpan(t *testing.T) {
	assertClean(t,
		`<span class="indent"><span style="--0:#C792EA">  </span></span>`,
		`<span class="indent">  </span>`)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: SVG removal
// <svg> elements (decorative icons, anchor indicators) MUST be removed entirely
// including all children. Surrounding structure stays intact.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_RemovesSVG(t *testing.T) {
	assertClean(t,
		`<svg width="16" height="16"><path d="M1 2L3 4"></path></svg>`,
		``)
}

func TestCleanHTML_RemovesSVGInsideAnchor(t *testing.T) {
	// Starlight heading anchor pattern — SVG removed, anchor kept.
	assertClean(t,
		`<a href="#foo"><svg width="16" height="16"><path d="M1 2"></path></svg>Link</a>`,
		`<a href="#foo">Link</a>`)
}

func TestCleanHTML_RemovesSVGPreservesAdjacentText(t *testing.T) {
	assertClean(t,
		`<p>before<svg viewBox="0 0 24 24"><circle r="5"></circle></svg>after</p>`,
		`<p>beforeafter</p>`)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: aria-hidden removal
// Elements with aria-hidden="true" are explicitly marked as non-content by the
// page author. They MUST be removed entirely including all children.
// aria-hidden="false" or absent → element is kept.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_RemovesAriaHiddenTrue(t *testing.T) {
	assertClean(t,
		`<span aria-hidden="true">decorative</span>`,
		``)
}

func TestCleanHTML_KeepsAriaHiddenFalse(t *testing.T) {
	in := `<span aria-hidden="false">visible</span>`
	assertClean(t, in, in)
}

func TestCleanHTML_RemovesAriaHiddenIconSpan(t *testing.T) {
	// Starlight anchor icon pattern
	assertClean(t,
		`<span aria-hidden="true" class="sl-anchor-icon"><svg width="16" height="16"><path d="M1 2"></path></svg></span>`,
		``)
}

func TestCleanHTML_KeepsAdjacentContentWhenAriaHiddenRemoved(t *testing.T) {
	assertClean(t,
		`<div><span aria-hidden="true">icon</span>Real content</div>`,
		`<div>Real content</div>`)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Interactive element removal
// <button>, <script>, <noscript> carry no content value for agents.
// They MUST be removed entirely.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_RemovesButton(t *testing.T) {
	assertClean(t,
		`<button title="Copy to clipboard" data-code="x=1">Copy</button>`,
		``)
}

func TestCleanHTML_RemovesButtonPreservesContext(t *testing.T) {
	assertClean(t,
		`<div class="code-block"><pre><code>x = 1</code></pre><button>Copy</button></div>`,
		`<div class="code-block"><pre><code>x = 1</code></pre></div>`)
}

func TestCleanHTML_RemovesScript(t *testing.T) {
	assertClean(t,
		`<script>console.log("hi")</script>`,
		``)
}

func TestCleanHTML_RemovesNoscript(t *testing.T) {
	assertClean(t,
		`<noscript>Enable JS</noscript>`,
		``)
}

func TestCleanHTML_RemovesScriptBetweenContent(t *testing.T) {
	assertClean(t,
		`<p>before</p><script>x()</script><p>after</p>`,
		`<p>before</p><p>after</p>`)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Empty element removal
// After all other cleaning, elements with no visible text and no child elements
// MUST be removed. Void elements (br, hr, img) are always kept.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_RemovesEmptyDiv(t *testing.T) {
	assertClean(t, `<div></div>`, ``)
}

func TestCleanHTML_RemovesWhitespaceOnlyDiv(t *testing.T) {
	assertClean(t, `<div>   </div>`, ``)
}

func TestCleanHTML_RemovesDivEmptiedByCleanup(t *testing.T) {
	// Inner button removed → parent div becomes empty → also removed.
	assertClean(t,
		`<div><button>Copy</button></div>`,
		``)
}

func TestCleanHTML_RemovesAriaLivePoliteDiv(t *testing.T) {
	// Common Starlight pattern: empty live region placeholder.
	assertClean(t, `<div aria-live="polite"></div>`, ``)
}

func TestCleanHTML_KeepsDivWithText(t *testing.T) {
	in := `<div>content</div>`
	assertClean(t, in, in)
}

func TestCleanHTML_KeepsDivWithChildElement(t *testing.T) {
	in := `<div><p>text</p></div>`
	assertClean(t, in, in)
}

func TestCleanHTML_KeepsBr(t *testing.T) {
	in := `<br/>`
	assertClean(t, in, in)
}

func TestCleanHTML_KeepsHr(t *testing.T) {
	in := `<hr/>`
	assertClean(t, in, in)
}

func TestCleanHTML_KeepsImg(t *testing.T) {
	in := `<img src="x.png"/>`
	assertClean(t, in, in)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Framework hash class stripping
// Auto-generated class names (astro-XXXX) carry no semantic meaning and MUST
// be stripped. If stripping removes all classes, the class attribute itself is
// removed. Non-hash classes are preserved.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_StripsAstroHashClass(t *testing.T) {
	assertClean(t,
		`<div class="astro-7nkwcw3z">text</div>`,
		`<div>text</div>`)
}

func TestCleanHTML_StripsAstroKeepsSemanticClass(t *testing.T) {
	assertClean(t,
		`<h1 class="astro-j6tvhyss page-title">Title</h1>`,
		`<h1 class="page-title">Title</h1>`)
}

func TestCleanHTML_StripsMultipleAstroClasses(t *testing.T) {
	assertClean(t,
		`<div class="content-panel astro-7nkwcw3z astro-abc123">text</div>`,
		`<div class="content-panel">text</div>`)
}

func TestCleanHTML_KeepsNonHashClasses(t *testing.T) {
	in := `<div class="sl-markdown-content">text</div>`
	assertClean(t, in, in)
}

func TestCleanHTML_DoesNotStripAstroPrefix(t *testing.T) {
	// "astro-config" is a meaningful name, not a hash — no alphanumeric-only suffix.
	in := `<div class="astro-config">text</div>`
	assertClean(t, in, in)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Text fidelity
// The visible text content MUST be identical before and after cleaning.
// No characters may be added, removed, reordered, or entity-encoded
// beyond what the HTML parser normalizes.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_PreservesSpecialCharsInText(t *testing.T) {
	assertClean(t,
		`<span style="c:1">a &gt; b &amp; c</span>`,
		`a &gt; b &amp; c`)
}

func TestCleanHTML_PreservesWhitespace(t *testing.T) {
	assertClean(t,
		`<span style="a">  leading</span><span style="b">  trailing  </span>`,
		`  leading  trailing  `)
}

func TestCleanHTML_PreservesNewlines(t *testing.T) {
	assertClean(t,
		"<span style=\"a\">line1\nline2</span>",
		"line1\nline2")
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Structural integrity
// Non-span elements, sibling order, and parent-child relationships
// (outside of unwrapped/removed nodes) MUST remain unchanged.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_PreservesDivStructure(t *testing.T) {
	in := `<div class="ec-line"><div class="code">text</div></div>`
	assertClean(t, in, in)
}

func TestCleanHTML_PreservesSiblingOrder(t *testing.T) {
	assertClean(t,
		`<span style="a">1</span><span style="b">2</span><span style="c">3</span>`,
		`123`)
}

func TestCleanHTML_PreservesMixedContent(t *testing.T) {
	assertClean(t,
		`before<span style="a">middle</span>after`,
		`beforemiddleafter`)
}

func TestCleanHTML_DoesNotUnwrapNonSpanElements(t *testing.T) {
	in := `<em>emphasized</em>`
	assertClean(t, in, in)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Realistic syntax highlighter output
// Full multi-line examples from real syntax highlighting libraries.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_ExpressiveCodeLine(t *testing.T) {
	in := `<div class="ec-line"><div class="code">` +
		`<span style="--0:#C792EA">import</span>` +
		`<span style="--0:#D6DEEB"> { query } </span>` +
		`<span style="--0:#C792EA">from</span>` +
		`<span style="--0:#D6DEEB"> </span>` +
		`<span style="--0:#D9F5DD">"</span>` +
		`<span style="--0:#ECC48D">./_generated/server</span>` +
		`<span style="--0:#D9F5DD">"</span>` +
		`</div></div>`
	// html.Render encodes " as &#34; in text nodes — standard HTML serialization.
	want := `<div class="ec-line"><div class="code">` +
		`import { query } from &#34;./_generated/server&#34;` +
		`</div></div>`
	assertClean(t, in, want)
}

func TestCleanHTML_ExpressiveCodeWithIndentSpan(t *testing.T) {
	in := `<div class="ec-line"><div class="code">` +
		`<span class="indent"><span style="--0:#C792EA">  </span></span>` +
		`<span style="--0:#C792EA">args: {</span>` +
		`</div></div>`
	want := `<div class="ec-line"><div class="code">` +
		`<span class="indent">  </span>` +
		`args: {` +
		`</div></div>`
	assertClean(t, in, want)
}

func TestCleanHTML_PrismTokenSpans(t *testing.T) {
	in := `<span class="token keyword" style="color:#ff79c6">function</span>` +
		`<span class="token punctuation" style="color:#f8f8f2">(</span>` +
		`<span class="token punctuation" style="color:#f8f8f2">)</span>`
	want := `<span class="token keyword">function</span>` +
		`<span class="token punctuation">(</span>` +
		`<span class="token punctuation">)</span>`
	assertClean(t, in, want)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Realistic page-level structures
// Full patterns from Starlight/Astro doc sites combining multiple noise types.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_StarlightHeadingAnchor(t *testing.T) {
	// Real Starlight heading: hash class + anchor with aria-hidden SVG + sr-only span.
	in := `<div class="sl-heading-wrapper level-h2">` +
		`<h2 id="my-heading">My heading</h2>` +
		`<a class="sl-anchor-link" href="#my-heading">` +
		`<span aria-hidden="true" class="sl-anchor-icon">` +
		`<svg width="16" height="16"><path d="M1 2"></path></svg>` +
		`</span>` +
		`<span class="sr-only">Section titled &#34;My heading&#34;</span>` +
		`</a>` +
		`</div>`
	want := `<div class="sl-heading-wrapper level-h2">` +
		`<h2 id="my-heading">My heading</h2>` +
		`<a class="sl-anchor-link" href="#my-heading">` +
		`<span class="sr-only">Section titled &#34;My heading&#34;</span>` +
		`</a>` +
		`</div>`
	assertClean(t, in, want)
}

func TestCleanHTML_StarlightAsideWithIcon(t *testing.T) {
	in := `<aside aria-label="Tip" class="starlight-aside starlight-aside--tip">` +
		`<p class="starlight-aside__title" aria-hidden="true">` +
		`<svg aria-hidden="true" width="16" height="16"><path d="M1 2"></path></svg>Tip` +
		`</p>` +
		`<div class="starlight-aside__content"><p>Useful tip here.</p></div>` +
		`</aside>`
	// The aria-hidden <p> (title with icon) is removed; the content stays.
	want := `<aside aria-label="Tip" class="starlight-aside starlight-aside--tip">` +
		`<div class="starlight-aside__content"><p>Useful tip here.</p></div>` +
		`</aside>`
	assertClean(t, in, want)
}

func TestCleanHTML_CodeBlockWithCopyButton(t *testing.T) {
	// Starlight code block: code + copy button + empty live region.
	in := `<figure>` +
		`<pre><code>const x = 1;</code></pre>` +
		`<div class="copy">` +
		`<div aria-live="polite"></div>` +
		`<button title="Copy" data-code="const x = 1;">Copy</button>` +
		`</div>` +
		`</figure>`
	// Button removed, empty live div removed, empty copy div removed.
	want := `<figure>` +
		`<pre><code>const x = 1;</code></pre>` +
		`</figure>`
	assertClean(t, in, want)
}

func TestCleanHTML_AstroContentPanel(t *testing.T) {
	in := `<div class="content-panel astro-7nkwcw3z">` +
		`<div class="sl-container astro-7nkwcw3z">` +
		`<p>Real content here.</p>` +
		`</div></div>`
	want := `<div class="content-panel">` +
		`<div class="sl-container">` +
		`<p>Real content here.</p>` +
		`</div></div>`
	assertClean(t, in, want)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Safety / no-op cases
// CleanHTML MUST be safe on already-clean HTML, empty strings, and plain text.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_NoOpOnCleanHTML(t *testing.T) {
	in := `<div class="content"><p>Hello world</p></div>`
	assertClean(t, in, in)
}

func TestCleanHTML_NoOpOnPlainText(t *testing.T) {
	assertClean(t, `just plain text`, `just plain text`)
}

func TestCleanHTML_EmptyString(t *testing.T) {
	assertClean(t, "", "")
}

func TestCleanHTML_EmptySpan(t *testing.T) {
	assertClean(t, `<span style="a"></span>`, ``)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Spec: Edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestCleanHTML_DeeplyNestedBareSpans(t *testing.T) {
	assertClean(t, `<span><span><span>deep</span></span></span>`, `deep`)
}

func TestCleanHTML_SpanWithMultipleChildren(t *testing.T) {
	assertClean(t,
		`<div><span style="a"><em>a</em><strong>b</strong></span></div>`,
		`<div><em>a</em><strong>b</strong></div>`)
}

func TestCleanHTML_SelfClosingElementsPreserved(t *testing.T) {
	assertClean(t,
		`<br/><hr/><img src="x.png"/>`,
		`<br/><hr/><img src="x.png"/>`)
}

func TestCleanHTML_AnchorTagPreserved(t *testing.T) {
	assertClean(t,
		`<a href="/foo" style="color:blue">link</a>`,
		`<a href="/foo">link</a>`)
}

func TestCleanHTML_CascadingRemoval(t *testing.T) {
	// Inner content all removed → parent becomes empty → also removed.
	assertClean(t,
		`<div><span aria-hidden="true">icon</span><script>x()</script></div>`,
		``)
}

// --- helper ---

func assertClean(t *testing.T, input, want string) {
	t.Helper()
	got := CleanHTML(input)
	if got != want {
		t.Errorf("CleanHTML mismatch:\n  input: %s\n  want:  %s\n  got:   %s", input, want, got)
	}
}
