package ui

import (
	"regexp"
	"testing"
	"time"
	_ "time/tzdata"
)

// Every preset the page offers must be accepted when the plugin validates the
// saved config, otherwise picking it would make CPA reject the reconfigure.
func TestPresetTimezonesLoad(t *testing.T) {
	block := regexp.MustCompile(`(?s)const TZ_NAMES = \{(.*?)\};\s*const TZ_GROUPS = \[(.*?)\];`).FindSubmatch(IndexHTML)
	if block == nil {
		t.Fatal("TZ_NAMES / TZ_GROUPS not found in index.html")
	}
	zone := regexp.MustCompile(`"((?:UTC)|(?:[A-Z][A-Za-z_]+/[A-Za-z_/]+))"`)
	seen := map[string]bool{}
	for _, part := range block[1:] {
		for _, m := range zone.FindAllSubmatch(part, -1) {
			seen[string(m[1])] = true
		}
	}
	if len(seen) < 60 {
		t.Fatalf("expected at least 60 preset zones, found %d", len(seen))
	}
	for z := range seen {
		if _, err := time.LoadLocation(z); err != nil {
			t.Errorf("preset %q is not a valid Go timezone: %v", z, err)
		}
	}
}
