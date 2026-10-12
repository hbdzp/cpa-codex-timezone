package rewrite

import (
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"cpa-codex-timezone/internal/config"
)

// 2026-10-12 23:30 UTC is already 2026-10-13 in Asia/Shanghai.
var fixedNow = time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC)

func settings(t *testing.T, f config.File) config.Settings {
	t.Helper()
	s, err := config.FromFile(f)
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	return s
}

func boolPtr(b bool) *bool { return &b }

const codexBody = `{
  "model": "gpt-5.5",
  "instructions": "You are Codex.",
  "input": [
    {"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/repo</cwd>\n  <current_date>2026-10-12</current_date>\n  <timezone>America/Los_Angeles</timezone>\n</environment_context>"}]},
    {"type":"message","role":"assistant","content":[{"type":"output_text","text":"<timezone>Keep/Me</timezone>"}]},
    {"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
  ]
}`

func TestApplyReplacesEnvironmentContext(t *testing.T) {
	s := settings(t, config.File{Timezone: "Asia/Shanghai"})
	res, err := Apply([]byte(codexBody), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	text := gjson.GetBytes(res.Body, "input.0.content.0.text").String()
	if !strings.Contains(text, "<timezone>Asia/Shanghai</timezone>") {
		t.Errorf("timezone not replaced: %s", text)
	}
	if !strings.Contains(text, "<current_date>2026-10-13</current_date>") {
		t.Errorf("date not computed in target timezone: %s", text)
	}
	if got := gjson.GetBytes(res.Body, "input.1.content.0.text").String(); got != "<timezone>Keep/Me</timezone>" {
		t.Errorf("assistant message must be untouched, got %q", got)
	}
	if res.Injected {
		t.Error("if_missing must not inject when a timezone tag exists")
	}
	if res.TimezoneReplaced != 1 || res.DateReplaced != 1 || !res.Changed {
		t.Errorf("unexpected result %+v", res)
	}
	if n := len(gjson.GetBytes(res.Body, "input").Array()); n != 3 {
		t.Errorf("input length changed to %d", n)
	}
}

func TestApplyKeepsDateWhenDisabled(t *testing.T) {
	s := settings(t, config.File{Timezone: "Asia/Tokyo", RewriteCurrentDate: boolPtr(false)})
	res, err := Apply([]byte(codexBody), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	text := gjson.GetBytes(res.Body, "input.0.content.0.text").String()
	if !strings.Contains(text, "<current_date>2026-10-12</current_date>") || !strings.Contains(text, "<timezone>Asia/Tokyo</timezone>") {
		t.Errorf("unexpected text: %s", text)
	}
}

func TestApplyHandlesUnavailableAndStringContent(t *testing.T) {
	body := `{"instructions":"<environment_context><timezone status=\"unavailable\" /></environment_context>","input":[{"role":"user","content":"<environment_context><current_date status=\"unavailable\" /></environment_context>"}]}`
	s := settings(t, config.File{Timezone: "Europe/Berlin"})
	res, err := Apply([]byte(body), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := gjson.GetBytes(res.Body, "instructions").String(); got != "<environment_context><timezone>Europe/Berlin</timezone></environment_context>" {
		t.Errorf("instructions = %q", got)
	}
	if got := gjson.GetBytes(res.Body, "input.0.content").String(); got != "<environment_context><current_date>2026-10-13</current_date></environment_context>" {
		t.Errorf("content = %q", got)
	}
}

func TestApplyIgnoresTagsOutsideEnvironmentContext(t *testing.T) {
	prompt := "Fix this XML: <timezone>UTC</timezone> <current_date>2020-01-01</current_date>"
	body := `{"input":[{"role":"user","content":[{"type":"input_text","text":"<environment_context>\n<timezone>America/Los_Angeles</timezone>\n</environment_context>\n` +
		strings.ReplaceAll(prompt, `"`, `\"`) + `"}]}]}`
	s := settings(t, config.File{Timezone: "Asia/Tokyo"})
	res, err := Apply([]byte(body), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	text := gjson.GetBytes(res.Body, "input.0.content.0.text").String()
	if !strings.HasPrefix(text, "<environment_context>\n<timezone>Asia/Tokyo</timezone>\n</environment_context>") {
		t.Errorf("environment_context not rewritten: %q", text)
	}
	if !strings.HasSuffix(text, prompt) {
		t.Errorf("user prompt must be untouched: %q", text)
	}
	if res.TimezoneReplaced != 1 || res.DateReplaced != 0 {
		t.Errorf("unexpected counts %+v", res)
	}
}

func TestApplyInjectsWhenMissing(t *testing.T) {
	body := `{"model":"gpt-5.5","instructions":"x","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"what time is it?"}]}]}`
	s := settings(t, config.File{Timezone: "Asia/Shanghai", InjectTemplate: "TZ={tz} NOW={now}"})
	res, err := Apply([]byte(body), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Injected {
		t.Fatal("expected injection")
	}
	last := gjson.GetBytes(res.Body, "input.1")
	if last.Get("role").String() != "developer" {
		t.Errorf("injected role = %q", last.Get("role").String())
	}
	if got := last.Get("content.0.text").String(); got != "TZ=Asia/Shanghai NOW=2026-10-13 07:30:00 +08:00" {
		t.Errorf("injected text = %q", got)
	}
	if gjson.GetBytes(res.Body, "instructions").String() != "x" {
		t.Error("instructions must not change")
	}
}

func TestApplyInjectModes(t *testing.T) {
	cases := []struct {
		inject string
		body   string
		want   bool
	}{
		{config.InjectOff, `{"input":[]}`, false},
		{config.InjectAlways, codexBody, true},
		{config.InjectIfMissing, `{"input":"hello"}`, true},
		{config.InjectIfMissing, `{}`, true},
	}
	for _, tc := range cases {
		s := settings(t, config.File{Inject: tc.inject})
		res, err := Apply([]byte(tc.body), s, fixedNow)
		if err != nil {
			t.Fatalf("%s: %v", tc.inject, err)
		}
		if res.Injected != tc.want {
			t.Errorf("inject=%s body=%s: injected=%v want %v", tc.inject, tc.body, res.Injected, tc.want)
		}
		if res.Injected && gjson.GetBytes(res.Body, "input.@reverse.0.role").String() != "developer" {
			t.Errorf("inject=%s: last input item is not the developer note: %s", tc.inject, res.Body)
		}
	}
}

func TestApplyStringInputPreservesUserText(t *testing.T) {
	s := settings(t, config.File{})
	res, err := Apply([]byte(`{"input":"hello"}`), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := gjson.GetBytes(res.Body, "input.0.content.0.text").String(); got != "hello" {
		t.Errorf("user text = %q", got)
	}
}

func TestApplyRejectsNonObject(t *testing.T) {
	s := settings(t, config.File{})
	for _, body := range []string{"", "[]", "not json"} {
		res, err := Apply([]byte(body), s, fixedNow)
		if err == nil {
			t.Errorf("%q: expected error", body)
		}
		if string(res.Body) != body {
			t.Errorf("%q: body must be returned unchanged", body)
		}
	}
}

func TestApplyNoopWhenNothingToDo(t *testing.T) {
	s := settings(t, config.File{Inject: config.InjectOff})
	body := `{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`
	res, err := Apply([]byte(body), s, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || string(res.Body) != body {
		t.Errorf("expected unchanged body, got %s", res.Body)
	}
}
