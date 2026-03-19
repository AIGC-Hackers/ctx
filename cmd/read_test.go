package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn and returns what it wrote to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestReadOutput_SummaryUsesTarget(t *testing.T) {
	// Build content long enough to trigger structural summary (>2000 lines)
	var b strings.Builder
	b.WriteString("# Section One\n\n")
	for i := 0; i < 2100; i++ {
		b.WriteString("Line of content.\n")
	}
	content := b.String()

	cmd := &ReadCmd{URL: ""} // empty — URL came from -d body
	target := "https://from-data-body.com/docs"

	out := captureStdout(t, func() {
		cmd.output("/tmp/cache.md", target, content, true)
	})

	if !strings.Contains(out, "[ctx:summary]") {
		t.Fatal("expected structural summary for long document")
	}
	if !strings.Contains(out, target) {
		t.Errorf("summary should reference target URL %q, got:\n%s", target, out[:min(len(out), 200)])
	}
	if strings.Contains(out, "ctx read  -s") {
		t.Error("summary contains empty URL (double space), target was not passed through")
	}
}

func TestReadOutput_SummaryUsesExplicitURL(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Heading\n\n")
	for i := 0; i < 2100; i++ {
		b.WriteString("Content line.\n")
	}
	content := b.String()

	target := "https://explicit.com/page"
	cmd := &ReadCmd{URL: target}

	out := captureStdout(t, func() {
		cmd.output("/tmp/cache.md", target, content, true)
	})

	if !strings.Contains(out, "ctx read "+target+" -s") {
		t.Errorf("summary should contain navigation hint with URL, got:\n%s", out[:min(len(out), 200)])
	}
}

// ===== skillReferencesHint =====

func TestSkillReferencesHint_GitHubSkillWithRefs(t *testing.T) {
	target := "https://github.com/AvdLee/swift-testing-agent-skill/blob/main/swift-testing-expert/SKILL.md"
	content := "---\nname: swift-testing-expert\n---\n\n# Swift Testing\n\nSee references/fundamentals.md for details.\n"

	got := skillReferencesHint(target, content)
	if !strings.Contains(got, "[ctx:skill-references]") {
		t.Fatal("expected skill-references hint for GitHub SKILL.md with references/")
	}
	if !strings.Contains(got, "github://AvdLee/swift-testing-agent-skill@main/swift-testing-expert/references/<file>") {
		t.Errorf("hint should contain github:// base path, got:\n%s", got)
	}
}

func TestSkillReferencesHint_GitHubScheme(t *testing.T) {
	target := "github://owner/repo@v2/my-skill/SKILL.md"
	content := "See references/guide.md\n"

	got := skillReferencesHint(target, content)
	if !strings.Contains(got, "github://owner/repo@v2/my-skill/references/<file>") {
		t.Errorf("should handle github:// scheme, got:\n%s", got)
	}
}

func TestSkillReferencesHint_NotSkillMD(t *testing.T) {
	target := "https://github.com/owner/repo/blob/main/README.md"
	content := "Some content with references/ mentioned.\n"

	got := skillReferencesHint(target, content)
	if strings.Contains(got, "[ctx:skill-references]") {
		t.Error("should not trigger for non-SKILL.md files")
	}
}

func TestSkillReferencesHint_NoReferencesInContent(t *testing.T) {
	target := "https://github.com/owner/repo/blob/main/my-skill/SKILL.md"
	content := "---\nname: simple-skill\n---\n\nNo refs here.\n"

	got := skillReferencesHint(target, content)
	if strings.Contains(got, "[ctx:skill-references]") {
		t.Error("should not trigger when content has no references/ pattern")
	}
}

func TestSkillReferencesHint_NonGitHub(t *testing.T) {
	target := "https://example.com/skills/SKILL.md"
	content := "See references/foo.md\n"

	got := skillReferencesHint(target, content)
	if strings.Contains(got, "[ctx:skill-references]") {
		t.Error("should not trigger for non-GitHub URLs")
	}
}

func TestSkillReferencesHint_PreservesOriginalContent(t *testing.T) {
	target := "https://github.com/owner/repo/blob/main/skill/SKILL.md"
	content := "Original content.\nSee references/foo.md\n"

	got := skillReferencesHint(target, content)
	if !strings.HasPrefix(got, content) {
		t.Error("hint should be appended, not replace original content")
	}
}

func TestReadOutput_ShortDocNeverSummarizes(t *testing.T) {
	content := "# Hello\n\nShort content.\n"
	cmd := &ReadCmd{URL: ""}

	out := captureStdout(t, func() {
		cmd.output("/tmp/cache.md", "https://example.com", content, true)
	})

	if strings.Contains(out, "[ctx:summary]") {
		t.Error("short doc should not produce structural summary")
	}
	if out != content {
		t.Errorf("short doc should be printed as-is, got:\n%s", out)
	}
}
