package config

import "testing"

func TestParseYAMLDefaults(t *testing.T) {
	s, err := ParseYAML([]byte("enabled: true\npriority: 10\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled || s.Timezone != DefaultTimezone || s.Inject != InjectIfMissing || !s.RewriteCurrentDate {
		t.Errorf("unexpected defaults: %+v", s)
	}
	if s.InjectTemplate != DefaultInjectTemplate || len(s.Models) != 1 || s.Models[0] != "gpt-*" {
		t.Errorf("unexpected defaults: %+v", s)
	}
}

func TestParseYAMLFull(t *testing.T) {
	raw := `
enabled: true
timezone: Europe/London
models: ["gpt-5.5", " codex-* ", ""]
rewrite_current_date: false
inject: ALWAYS
inject_template: "tz={tz}"
`
	s, err := ParseYAML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if s.Timezone != "Europe/London" || s.Location.String() != "Europe/London" {
		t.Errorf("timezone = %q", s.Timezone)
	}
	if s.RewriteCurrentDate || s.Inject != InjectAlways || s.InjectTemplate != "tz={tz}" {
		t.Errorf("unexpected settings: %+v", s)
	}
	if len(s.Models) != 2 || s.Models[1] != "codex-*" {
		t.Errorf("models = %v", s.Models)
	}
}

func TestParseYAMLErrors(t *testing.T) {
	for _, raw := range []string{
		"timezone: Mars/Olympus",
		"inject: sometimes",
		"models: {",
	} {
		if _, err := ParseYAML([]byte(raw)); err == nil {
			t.Errorf("%q: expected error", raw)
		}
	}
}

func TestMatchesModel(t *testing.T) {
	s, err := FromFile(File{Models: []string{"gpt-*", "o3", "*codex*"}})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"gpt-5.5":          true,
		"GPT-5":            true,
		"o3":               true,
		"o3-mini":          false,
		"gpt-5.1-codex":    true,
		"my-codex-preview": true,
		"claude-opus":      false,
		"":                 false,
		"xgpt-5":           false,
	}
	for model, want := range cases {
		if got := s.MatchesModel(model); got != want {
			t.Errorf("MatchesModel(%q) = %v, want %v", model, got, want)
		}
	}
}
