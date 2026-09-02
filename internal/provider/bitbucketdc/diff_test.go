package bitbucketdc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlexIntStringAndNumber(t *testing.T) {
	var raw dcDiffResponse
	payload := `{
  "diffs": [{
    "hunks": [{
      "sourceLine": "10",
      "destinationLine": 12,
      "segments": [{
        "type": "ADDED",
        "lines": [
          {"line": "13", "source": 0, "destination": "13", "text": "hello"},
          {"line": 14, "text": "world"}
        ]
      }]
    }]
  }]
}`
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Diffs[0].Hunks[0].SourceLine.Int() != 10 {
		t.Fatalf("sourceLine %d", raw.Diffs[0].Hunks[0].SourceLine.Int())
	}
	if raw.Diffs[0].Hunks[0].Segments[0].Lines[0].Line.Int() != 13 {
		t.Fatalf("line %d", raw.Diffs[0].Hunks[0].Segments[0].Lines[0].Line.Int())
	}
	unified := dcDiffToUnified("a.go", raw)
	if !strings.Contains(unified, "+hello") || !strings.Contains(unified, "@@ -10 +12 @@") {
		t.Fatalf("unified:\n%s", unified)
	}
}
