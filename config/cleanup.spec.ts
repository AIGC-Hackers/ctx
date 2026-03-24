import { describe, it, expect } from "bun:test";
import { readFileSync } from "fs";
import { join } from "path";
import { Window } from "happy-dom";

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

const raw = readFileSync(join(import.meta.dir, "cleanup.js"), "utf-8");

// Extract the IIFE body so we can call it with an explicit `document` param.
// This avoids global pollution and gives each test an isolated DOM.
const fnBody = raw.slice(raw.indexOf("{") + 1, raw.lastIndexOf("}"));
const runScript = new Function("document", fnBody) as (doc: Document) => void;

/** Create a fresh DOM from raw body HTML, run the cleanup, return body innerHTML. */
function cleanup(bodyHTML: string): string {
  const window = new Window({ url: "https://test.local" });
  const doc = window.document;
  doc.body.innerHTML = bodyHTML;
  runScript(doc as unknown as Document);
  const result = doc.body.innerHTML;
  window.close();
  return result;
}

/** Generate filler text of approximately n characters. */
function filler(n: number): string {
  const word = "lorem ipsum dolor sit amet ";
  return word.repeat(Math.ceil(n / word.length)).slice(0, n);
}

// ---------------------------------------------------------------------------
// Phase 1 — Semantic container selection
// ---------------------------------------------------------------------------

describe("Phase 1: Semantic container", () => {
  it("should use <main> and discard surrounding chrome", () => {
    const result = cleanup(`
      <nav>Site navigation</nav>
      <main>
        <h1>Page Title</h1>
        <p>${filler(300)}</p>
      </main>
      <footer>Site footer</footer>
    `);
    expect(result).toContain("Page Title");
    expect(result).toContain("lorem");
    expect(result).not.toContain("Site navigation");
    expect(result).not.toContain("Site footer");
  });

  it("should use <article> when no <main> exists", () => {
    const result = cleanup(`
      <nav>Nav</nav>
      <article>
        <h2>Article Heading</h2>
        <p>${filler(300)}</p>
      </article>
      <footer>Footer</footer>
    `);
    expect(result).toContain("Article Heading");
    expect(result).not.toContain("Nav");
    expect(result).not.toContain("Footer");
  });

  it("should prefer [role=main] over <main> and <article>", () => {
    const result = cleanup(`
      <div role="main">
        <p>Role main wins ${filler(300)}</p>
      </div>
      <main>
        <p>Main loses ${filler(300)}</p>
      </main>
      <article>
        <p>Article loses ${filler(300)}</p>
      </article>
    `);
    expect(result).toContain("Role main wins");
    expect(result).not.toContain("Main loses");
    expect(result).not.toContain("Article loses");
  });

  it("should prefer <main> over <article>", () => {
    const result = cleanup(`
      <main>
        <p>Main wins ${filler(300)}</p>
      </main>
      <article>
        <p>Article loses ${filler(300)}</p>
      </article>
    `);
    expect(result).toContain("Main wins");
    expect(result).not.toContain("Article loses");
  });

  it("should clean nav/aside/sidebar noise WITHIN the semantic container", () => {
    const result = cleanup(`
      <main>
        <h1>Content</h1>
        <p>${filler(300)}</p>
        <nav>In-page nav</nav>
        <aside>Sidebar note</aside>
        <div class="sidebar-widget">Widget</div>
        <div class="menu-toggle">Menu</div>
        <div role="navigation">Role nav</div>
      </main>
    `);
    expect(result).toContain("Content");
    expect(result).not.toContain("In-page nav");
    expect(result).not.toContain("Sidebar note");
    expect(result).not.toContain("Widget");
    expect(result).not.toContain("Menu");
    expect(result).not.toContain("Role nav");
  });

  it("should preserve non-junk elements within semantic container", () => {
    const result = cleanup(`
      <main>
        <h1>Title</h1>
        <p>Paragraph one ${filler(200)}</p>
        <div class="info-box">Important info</div>
        <pre><code>const x = 42;</code></pre>
        <table><tr><td>Data</td></tr></table>
      </main>
    `);
    expect(result).toContain("Title");
    expect(result).toContain("Paragraph one");
    expect(result).toContain("Important info");
    expect(result).toContain("const x = 42;");
    expect(result).toContain("Data");
  });

  it("should fall through when semantic container has ≤200 chars", () => {
    // <main> exists but has too little text → Phase 1 skips
    const result = cleanup(`
      <main><p>Tiny</p></main>
      <div>
        <h1>Real content area</h1>
        <p>${filler(500)}</p>
      </div>
    `);
    // Phase 2 or 3 handles it; either way, real content must survive
    expect(result).toContain("Real content area");
  });

  it("should fall through when no semantic container exists", () => {
    const result = cleanup(`
      <div class="page">
        <div class="content">
          <h1>Non-semantic page</h1>
          <p>${filler(500)}</p>
        </div>
      </div>
    `);
    expect(result).toContain("Non-semantic page");
  });
});

// ---------------------------------------------------------------------------
// Phase 2 — Guarded density heuristic
// ---------------------------------------------------------------------------

describe("Phase 2: Broadest content container", () => {
  it("should select the broadest div that is not link-heavy", () => {
    // No semantic containers. A content div and a link-cluster div.
    // Content div has more text → wins.
    const result = cleanup(`
      <div id="content">
        <h2>Content block</h2>
        <p>${filler(400)} <a href="#">one link</a></p>
      </div>
      <div id="link-heavy">
        <a href="#">L1</a> <a href="#">L2</a> <a href="#">L3</a>
        <a href="#">L4</a> <a href="#">L5</a> <a href="#">L6</a>
      </div>
    `);
    expect(result).toContain("Content block");
  });

  it("should prefer parent container over narrow child with higher density", () => {
    // content-area wraps everything (most text) → should win
    // over code-wrapper which has higher density but less text.
    const result = cleanup(`
      <div class="content-area">
        <h1>Documentation</h1>
        <p>${filler(400)} <a href="#">link</a></p>
        <div class="code-wrapper">
          <pre><code>${filler(300)}</code></pre>
        </div>
        <p>Second paragraph ${filler(200)} <a href="#">another</a></p>
      </div>
    `);
    expect(result).toContain("Documentation");
    expect(result).toContain("Second paragraph");
  });

  it("should filter out link-heavy navigation divs (<20 chars/link)", () => {
    // nav-div has many links and little text → density < 20 → filtered
    const result = cleanup(`
      <div class="nav-cluster">
        <a href="#">A</a> <a href="#">B</a> <a href="#">C</a>
        <a href="#">D</a> <a href="#">E</a> <a href="#">F</a>
        <a href="#">G</a> <a href="#">H</a> <a href="#">I</a>
        <a href="#">J</a>
      </div>
      <div class="content">
        <h1>Real Content</h1>
        <p>${filler(400)}</p>
      </div>
    `);
    expect(result).toContain("Real Content");
  });

  it("should reject elements covering <30% of body text", () => {
    const result = cleanup(`
      <div class="page-wrapper">
        <h1>Documentation</h1>
        <p>${filler(600)} <a href="#">link</a></p>
        <div class="small-aside"><p>${filler(100)}</p></div>
        <p>More content ${filler(400)} <a href="#">another</a></p>
      </div>
    `);
    // small-aside covers ~100/1100 ≈ 9% → rejected
    // page-wrapper covers ~100% → selected
    expect(result).toContain("Documentation");
    expect(result).toContain("More content");
  });

  it("should reject elements with ≤200 chars regardless", () => {
    const result = cleanup(`
      <span>Loose text ${filler(300)}</span>
      <div><p>${filler(150)}</p></div>
      <div><p>${filler(150)}</p></div>
    `);
    // Neither div qualifies (≤200 chars) → Phase 3 runs
    expect(result).toContain("Loose text");
  });

  it("should fall through to Phase 3 when no div/section qualifies", () => {
    const result = cleanup(`
      <nav>Navigation links</nav>
      <p>${filler(400)}</p>
      <ul><li>Item 1</li><li>Item 2</li></ul>
      <footer>Footer stuff</footer>
    `);
    // No divs → Phase 3 removes nav/footer
    expect(result).toContain("lorem");
    expect(result).toContain("Item 1");
    expect(result).not.toContain("Navigation links");
    expect(result).not.toContain("Footer stuff");
  });
});

// ---------------------------------------------------------------------------
// Phase 3 — Subtractive fallback
// ---------------------------------------------------------------------------

describe("Phase 3: Subtractive cleanup", () => {
  // Helper: builds a DOM with no semantic containers and no qualifying divs,
  // so Phase 1 and 2 both skip and Phase 3 runs.
  function phase3(bodyHTML: string): string {
    // Wrap in <span> blocks to avoid creating qualifying divs
    return cleanup(bodyHTML);
  }

  it("should remove semantic noise elements (nav, header, footer, aside)", () => {
    const result = phase3(`
      <nav>Nav content</nav>
      <header>Header content</header>
      <p>Keep this ${filler(50)}</p>
      <aside>Aside content</aside>
      <footer>Footer content</footer>
    `);
    expect(result).toContain("Keep this");
    expect(result).not.toContain("Nav content");
    expect(result).not.toContain("Header content");
    expect(result).not.toContain("Aside content");
    expect(result).not.toContain("Footer content");
  });

  it("should remove elements by ARIA role", () => {
    const result = phase3(`
      <div role="navigation">Role nav</div>
      <div role="banner">Role banner</div>
      <div role="contentinfo">Role contentinfo</div>
      <p>Preserved ${filler(50)}</p>
    `);
    expect(result).toContain("Preserved");
    expect(result).not.toContain("Role nav");
    expect(result).not.toContain("Role banner");
    expect(result).not.toContain("Role contentinfo");
  });

  it("should remove elements matching sidebar/menu/nav class patterns", () => {
    const result = phase3(`
      <div class="page-sidebar">Sidebar</div>
      <div class="dropdown-menu">Menu</div>
      <div class="nav-bar">Navbar</div>
      <div class="site-footer">Site footer</div>
      <div class="site-header">Site header</div>
      <p>Content ${filler(50)}</p>
    `);
    expect(result).toContain("Content");
    expect(result).not.toContain("Sidebar");
    expect(result).not.toContain("Menu");
    expect(result).not.toContain("Navbar");
    expect(result).not.toContain("Site footer");
    expect(result).not.toContain("Site header");
  });

  it("should remove elements matching sidebar/menu/nav ID patterns", () => {
    const result = phase3(`
      <div id="left-sidebar">Sidebar</div>
      <div id="main-menu">Menu</div>
      <div id="top-nav">Top nav</div>
      <p>Content ${filler(50)}</p>
    `);
    expect(result).toContain("Content");
    expect(result).not.toContain("Sidebar");
    expect(result).not.toContain("Menu");
    expect(result).not.toContain("Top nav");
  });

  it("should preserve all content elements that are not junk", () => {
    const result = phase3(`
      <nav>Remove me</nav>
      <p>Paragraph</p>
      <ul><li>List item</li></ul>
      <table><tr><td>Cell</td></tr></table>
      <pre><code>code()</code></pre>
      <blockquote>Quote</blockquote>
      <footer>Remove me too</footer>
    `);
    expect(result).toContain("Paragraph");
    expect(result).toContain("List item");
    expect(result).toContain("Cell");
    expect(result).toContain("code()");
    expect(result).toContain("Quote");
    expect(result).not.toContain("Remove me");
  });
});

// ---------------------------------------------------------------------------
// Regression: information loss prevention
// ---------------------------------------------------------------------------

describe("Regression: information loss prevention", () => {
  it("should preserve full content on code-heavy documentation pages", () => {
    // Simulates the convexfs.dev/guides/authn-authz/ structure:
    // <main> contains prose with links + multiple code blocks.
    // Old algorithm picked the longest code block div, losing everything else.
    const result = cleanup(`
      <nav><a href="/">Home</a> <a href="/docs">Docs</a></nav>
      <aside class="sidebar">
        <a href="/guide1">Guide 1</a>
        <a href="/guide2">Guide 2</a>
      </aside>
      <main>
        <h1>Authentication & Authorization</h1>
        <p>ConvexFS relies on the same conventions as any other project
           for <a href="/auth">authentication</a> and authorization.</p>
        <div class="code-block">
          <pre><code>export const listMyFiles = query({
  args: { prefix: v.optional(v.string()) },
  handler: async (ctx, args) => {
    const identity = await ctx.auth.getUserIdentity();
    if (!identity) throw new Error("Not authenticated");
    return await fs.list(ctx, { prefix: args.prefix });
  },
});</code></pre>
        </div>
        <h2>Access control for HTTP routes</h2>
        <p>Register routes with auth callbacks. See
           <a href="/cdn">CDN parameters</a> for details.</p>
        <div class="code-block">
          <pre><code>registerRoutes(http, components.fs, fs, {
  pathPrefix: "/fs",
  uploadAuth: async (ctx) => {
    return (await ctx.auth.getUserIdentity()) !== null;
  },
});</code></pre>
        </div>
        <h2>Signed URL security</h2>
        <p>The download endpoint returns a 302 redirect to a time-limited
           signed URL on the CDN. <a href="/config">Configure TTL</a>.</p>
      </main>
      <footer><a href="/about">About</a></footer>
    `);
    // ALL main content must be preserved
    // innerHTML entity-encodes '&' → '&amp;', so match on a non-ambiguous substring
    expect(result).toContain("Authorization");
    expect(result).toContain("listMyFiles");
    expect(result).toContain("Access control for HTTP routes");
    expect(result).toContain("registerRoutes");
    expect(result).toContain("Signed URL security");
    expect(result).toContain("302 redirect");
    // Chrome must be removed
    expect(result).not.toContain("Guide 1");
    expect(result).not.toContain("About");
  });

  it("should not select a single code block div over the real content", () => {
    // Specifically tests the old bug: a code-only div wins density scoring
    // because code has zero links → score = text_length / 1.
    const longCode = "x = " + filler(800);
    const result = cleanup(`
      <div class="content-area">
        <h1>Title</h1>
        <p>${filler(200)} <a href="#">ref 1</a> <a href="#">ref 2</a></p>
        <div class="code-wrapper"><pre><code>${longCode}</code></pre></div>
        <p>${filler(200)} <a href="#">ref 3</a></p>
        <h2>Section Two</h2>
        <p>${filler(200)} <a href="#">ref 4</a></p>
      </div>
    `);
    expect(result).toContain("Title");
    expect(result).toContain("Section Two");
    expect(result).toContain(longCode.slice(0, 50));
  });

  it("should preserve content when main area has many links", () => {
    // A content area dense with links (API reference, changelog).
    // Old algorithm penalized this heavily with text/link scoring.
    const result = cleanup(`
      <main>
        <h1>API Reference</h1>
        <ul>
          <li><a href="/a">methodA()</a> - Does A. ${filler(50)}</li>
          <li><a href="/b">methodB()</a> - Does B. ${filler(50)}</li>
          <li><a href="/c">methodC()</a> - Does C. ${filler(50)}</li>
          <li><a href="/d">methodD()</a> - Does D. ${filler(50)}</li>
          <li><a href="/e">methodE()</a> - Does E. ${filler(50)}</li>
          <li><a href="/f">methodF()</a> - Does F. ${filler(50)}</li>
          <li><a href="/g">methodG()</a> - Does G. ${filler(50)}</li>
          <li><a href="/h">methodH()</a> - Does H. ${filler(50)}</li>
          <li><a href="/i">methodI()</a> - Does I. ${filler(50)}</li>
          <li><a href="/j">methodJ()</a> - Does J. ${filler(50)}</li>
        </ul>
      </main>
    `);
    expect(result).toContain("API Reference");
    expect(result).toContain("methodA()");
    expect(result).toContain("methodJ()");
  });

  it("should never produce empty output", () => {
    // Even a degenerate page should retain something
    const result = cleanup(`<p>Hello world</p>`);
    expect(result.trim().length).toBeGreaterThan(0);
    expect(result).toContain("Hello world");
  });

  it("should handle a page with only a large code block and nothing else", () => {
    const code = filler(500);
    const result = cleanup(`<pre><code>${code}</code></pre>`);
    expect(result).toContain(code.slice(0, 50));
  });
});
