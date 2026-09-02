package bitbucketdc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestDCDiffLineIsContent(t *testing.T) {
	var raw dcDiffResponse
	// Bitbucket DC: "line" is content; numbers are source/destination.
	payload := `{
  "diffs": [{
    "hunks": [{
      "sourceLine": 10,
      "destinationLine": 12,
      "segments": [{
        "type": "ADDED",
        "lines": [
          {"source": 0, "destination": 13, "line": ".join('');"},
          {"source": 0, "destination": 14, "line": "hello world"}
        ]
      }]
    }]
  }]
}`
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatal(err)
	}
	ln := raw.Diffs[0].Hunks[0].Segments[0].Lines[0]
	if ln.Line != ".join('');" {
		t.Fatalf("content %q", ln.Line)
	}
	if ln.Destination.Int() != 13 {
		t.Fatalf("destination %d", ln.Destination.Int())
	}
	unified := dcDiffToUnified("a.js", raw)
	if !strings.Contains(unified, "+.join('');") || !strings.Contains(unified, "+hello world") {
		t.Fatalf("unified:\n%s", unified)
	}
	if !strings.Contains(unified, "@@ -10 +12 @@") {
		t.Fatalf("hunk header missing:\n%s", unified)
	}
}

func TestDCDiffFiltersOtherPaths(t *testing.T) {
	var raw dcDiffResponse
	payload := `{
  "diffs": [
    {
      "destination": {"toString": "keep.go"},
      "hunks": [{"sourceLine": 1, "destinationLine": 1, "segments": [
        {"type": "ADDED", "lines": [{"destination": 1, "line": "keep"}]}
      ]}]
    },
    {
      "destination": {"toString": "other.go"},
      "hunks": [{"sourceLine": 1, "destinationLine": 1, "segments": [
        {"type": "ADDED", "lines": [{"destination": 1, "line": "leak"}]}
      ]}]
    }
  ]
}`
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatal(err)
	}
	unified := dcDiffToUnified("keep.go", raw)
	if strings.Contains(unified, "leak") {
		t.Fatalf("leaked other file:\n%s", unified)
	}
	if !strings.Contains(unified, "+keep") {
		t.Fatalf("missing wanted file:\n%s", unified)
	}
}

func TestEncodeDiffPath(t *testing.T) {
	got := encodeDiffPath("src/foo bar.js")
	if got != "src/foo%20bar%2Ejs" {
		t.Fatalf("%q", got)
	}
	dot := encodeDiffPath("envs/.env.development")
	if dot != "envs/%2Eenv%2Edevelopment" {
		t.Fatalf("dotfile path: %q", dot)
	}
	q := encodeDiffQueryPath("envs/.env.development")
	if q != "envs%2F%2Eenv%2Edevelopment" {
		t.Fatalf("query path: %q", q)
	}
	urls := diffURLs("http://bb/rest/api/1.0/projects/P/repos/r/pull-requests/1", "envs/.env.development")
	if len(urls) < 2 {
		t.Fatalf("want fallbacks, got %v", urls)
	}
	if !strings.Contains(urls[0], "path=envs%2F%2Eenv%2Edevelopment") {
		t.Fatalf("query-first: %v", urls[0])
	}
	if !strings.Contains(urls[1], "/diff/envs/%2Eenv%2Edevelopment") {
		t.Fatalf("path form: %v", urls[1])
	}
}

func TestBuildDCAnchorMultiline(t *testing.T) {
	a := buildDCAnchor(&domain.Anchor{
		Path: "envs/.env", Side: domain.SideRight,
		Line: 10, EndLine: 18, LineType: domain.LineAdded,
	})
	if a["line"] != 18 {
		t.Fatalf("end line: %v", a["line"])
	}
	marker, ok := a["multilineMarker"].(map[string]any)
	if !ok || marker["startLine"] != 10 {
		t.Fatalf("marker: %v", a["multilineMarker"])
	}
	span, ok := a["multilineSpan"].(map[string]any)
	if !ok || span["dstSpanStart"] != 10 || span["dstSpanEnd"] != 18 {
		t.Fatalf("span: %v", a["multilineSpan"])
	}
	single := buildDCAnchor(&domain.Anchor{
		Path: "a.go", Side: domain.SideLeft, Line: 3, LineType: domain.LineRemoved,
	})
	if _, ok := single["multilineMarker"]; ok {
		t.Fatal("single-line should not set multilineMarker")
	}
	if single["fileType"] != "FROM" || single["line"] != 3 {
		t.Fatalf("%v", single)
	}
}

func TestDCCommentToDomainMultiline(t *testing.T) {
	cm := dcComment{
		ID: 1, Text: "range",
		Anchor: &dcCommentAnchor{
			Path: "a.go", Line: 18, LineType: "ADDED", FileType: "TO",
			MultilineMarker: &dcMultilineMarker{StartLine: 10, StartLineType: "ADDED"},
		},
	}
	c := cm.toDomain()
	if c.Anchor == nil || c.Anchor.Line != 10 || c.Anchor.EndLine != 18 {
		t.Fatalf("%+v", c.Anchor)
	}
}

func TestFlexIntNumericString(t *testing.T) {
	var n flexInt
	if err := json.Unmarshal([]byte(`"42"`), &n); err != nil {
		t.Fatal(err)
	}
	if n.Int() != 42 {
		t.Fatalf("%d", n.Int())
	}
}
