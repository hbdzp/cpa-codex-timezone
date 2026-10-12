package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

func call(t *testing.T, method string, payload any) envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	out, err := handleMethod(method, raw)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var env envelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func register(t *testing.T, yaml string) {
	t.Helper()
	env := call(t, pluginabi.MethodPluginRegister, lifecycleRequest{ConfigYAML: []byte(yaml)})
	if !env.OK {
		t.Fatalf("register failed: %+v", env.Error)
	}
}

func normalize(t *testing.T, req pluginapi.RequestTransformRequest) []byte {
	t.Helper()
	env := call(t, pluginabi.MethodRequestNormalize, req)
	var out pluginapi.PayloadResponse
	if err := json.Unmarshal(env.Result, &out); err != nil {
		t.Fatal(err)
	}
	return out.Body
}

const envBody = `{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context><timezone>UTC</timezone></environment_context>"}]}]}`

func TestRegisterDeclaresCapabilities(t *testing.T) {
	env := call(t, pluginabi.MethodPluginRegister, lifecycleRequest{})
	var reg map[string]any
	_ = json.Unmarshal(env.Result, &reg)
	caps := reg["capabilities"].(map[string]any)
	if caps["request_normalizer"] != true || caps["management_api"] != true {
		t.Errorf("capabilities = %v", caps)
	}
}

func TestRegisterRejectsInvalidTimezone(t *testing.T) {
	if _, err := handleMethod(pluginabi.MethodPluginReconfigure, []byte(`{"config_yaml":"dGltZXpvbmU6IE5vcGUvTm9wZQ=="}`)); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

func TestNormalizeRewritesOnlyMatchingCodexRequests(t *testing.T) {
	register(t, "enabled: true\ntimezone: Asia/Shanghai\ninject: off\n")

	got := normalize(t, pluginapi.RequestTransformRequest{ToFormat: "codex", Model: "gpt-5.5", Body: []byte(envBody)})
	if !strings.Contains(gjson.GetBytes(got, "input.0.content.0.text").String(), "<timezone>Asia/Shanghai</timezone>") {
		t.Errorf("codex gpt request not rewritten: %s", got)
	}

	for _, req := range []pluginapi.RequestTransformRequest{
		{ToFormat: "claude", Model: "gpt-5.5", Body: []byte(envBody)},
		{ToFormat: "codex", Model: "claude-opus-4", Body: []byte(envBody)},
	} {
		if got := normalize(t, req); string(got) != envBody {
			t.Errorf("%s/%s must pass through, got %s", req.ToFormat, req.Model, got)
		}
	}
}

func TestNormalizePassesThroughWhenDisabled(t *testing.T) {
	register(t, "enabled: false\ntimezone: Asia/Shanghai\n")
	got := normalize(t, pluginapi.RequestTransformRequest{ToFormat: "codex", Model: "gpt-5.5", Body: []byte(envBody)})
	if string(got) != envBody {
		t.Errorf("disabled plugin changed body: %s", got)
	}
}

func manage(t *testing.T, method, path, body string) (int, []byte) {
	t.Helper()
	env := call(t, pluginabi.MethodManagementHandle, pluginapi.ManagementRequest{Method: method, Path: path, Body: []byte(body)})
	var resp pluginapi.ManagementResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Body
}

func TestManagementRoutes(t *testing.T) {
	register(t, "enabled: true\ntimezone: Asia/Tokyo\n")
	nowFunc = func() time.Time { return time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = time.Now }()

	code, body := manage(t, "GET", resourceUIPath, "")
	if code != 200 || !strings.Contains(string(body), "<html") {
		t.Errorf("ui: %d %.80s", code, body)
	}

	code, body = manage(t, "GET", statusPath, "")
	if code != 200 || gjson.GetBytes(body, "settings.timezone").String() != "Asia/Tokyo" || gjson.GetBytes(body, "local_now").String() != "2026-10-13 01:00:00" {
		t.Errorf("status: %d %s", code, body)
	}

	preview := `{"body":` + mustQuote(envBody) + `,"settings":{"timezone":"Europe/Paris","inject":"off"}}`
	code, body = manage(t, "POST", previewPath, preview)
	if code != 200 || !gjson.GetBytes(body, "matched").Bool() ||
		!strings.Contains(gjson.GetBytes(body, "body.input.0.content.0.text").String(), "Europe/Paris") {
		t.Errorf("preview: %d %s", code, body)
	}

	code, _ = manage(t, "POST", previewPath, `{"body":"not json"}`)
	if code != 400 {
		t.Errorf("preview invalid body: %d", code)
	}
	code, _ = manage(t, "POST", previewPath, `{"body":{},"settings":{"timezone":"Bad/Zone"}}`)
	if code != 400 {
		t.Errorf("preview invalid timezone: %d", code)
	}
	code, _ = manage(t, "GET", "/v0/management/other", "")
	if code != 404 {
		t.Errorf("unknown route: %d", code)
	}
}

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
