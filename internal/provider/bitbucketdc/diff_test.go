package bitbucketdc

import (
	"encoding/json"
	"strings"
	"testing"
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

func TestFlexIntNumericString(t *testing.T) {
	var n flexInt
	if err := json.Unmarshal([]byte(`"42"`), &n); err != nil {
		t.Fatal(err)
	}
	if n.Int() != 42 {
		t.Fatalf("%d", n.Int())
	}
}
