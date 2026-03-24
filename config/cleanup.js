// DOM cleanup script injected via Cloudflare Browser Rendering API.
// Extracts main page content using a three-phase strategy where every
// failure mode degrades to "extra noise" rather than "lost content".
//
// Phase 1 — Semantic container: [role=main] / <main> / <article>
// Phase 2 — Broadest content div: among divs covering ≥30% of body text
//           and not link-heavy, pick the one with the MOST text.
// Phase 3 — Subtractive: remove known junk (nav, footer, sidebar…)
(function () {
  // Phase 1: prefer semantic containers — safest, covers most modern sites.
  var root =
    document.querySelector("[role=main]") ||
    document.querySelector("main") ||
    document.querySelector("article");
  if (root && (root.innerText || "").length > 200) {
    root
      .querySelectorAll(
        "nav,aside,[role=navigation],[class*=sidebar],[class*=menu]"
      )
      .forEach(function (e) {
        e.remove();
      });
    document.body.innerHTML = root.outerHTML;
    return;
  }

  // Phase 2: broadest content container with density guard.
  // Text length is the primary metric (prefers parent over narrow child).
  // Density (chars per link) is only a minimum filter to exclude nav clusters.
  var bodyLen = (document.body.innerText || "").length;
  var best = null,
    bestLen = 0;
  document.querySelectorAll("div,section").forEach(function (el) {
    var t = el.innerText || "";
    var links = el.querySelectorAll("a").length;
    // ≥30% coverage: not too narrow (filters single code block wrappers)
    // ≥20 chars/link: not a navigation cluster
    if (
      t.length > 200 &&
      t.length / bodyLen > 0.3 &&
      t.length / (links + 1) > 20 &&
      t.length > bestLen
    ) {
      bestLen = t.length;
      best = el;
    }
  });
  if (best) {
    document.body.innerHTML = best.outerHTML;
    return;
  }

  // Phase 3: subtractive fallback — remove known junk, keep everything else.
  document
    .querySelectorAll(
      "nav,header,footer,aside," +
        "[role=navigation],[role=banner],[role=contentinfo]," +
        "[class*=sidebar],[class*=menu],[class*=nav-],[class*=footer]," +
        "[class*=header],[id*=sidebar],[id*=menu],[id*=nav]"
    )
    .forEach(function (e) {
      e.remove();
    });
})();
