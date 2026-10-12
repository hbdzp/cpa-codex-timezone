package main

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"

	"cpa-codex-timezone/internal/config"
	"cpa-codex-timezone/internal/rewrite"
)

const (
	pluginID     = "codex-timezone"
	targetFormat = "codex"
)

// CPA refuses to load plugins with empty metadata fields. Override with
// -ldflags "-X main.pluginRepository=https://github.com/you/your-fork".
var (
	pluginVersion    = "0.1.0"
	pluginAuthor     = "hbdzp"
	pluginRepository = "https://github.com/hbdzp/cpa-codex-timezone"
)

var (
	current atomic.Pointer[config.Settings]
	stats   counters
	nowFunc = time.Now
)

type counters struct {
	Seen      atomic.Int64
	Matched   atomic.Int64
	Rewritten atomic.Int64
	Injected  atomic.Int64
	Errors    atomic.Int64
	LastModel atomic.Value
	LastAt    atomic.Int64
}

func init() {
	s := config.Default()
	current.Store(&s)
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type registration struct {
	SchemaVersion uint32             `json:"schema_version"`
	Metadata      pluginapi.Metadata `json:"metadata"`
	Capabilities  capabilities       `json:"capabilities"`
}

type capabilities struct {
	RequestNormalizer bool `json:"request_normalizer"`
	ManagementAPI     bool `json:"management_api"`
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		if err := configure(request); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodRequestNormalize:
		return normalizeRequest(request)
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func configure(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
	}
	s, err := config.ParseYAML(req.ConfigYAML)
	if err != nil {
		return err
	}
	current.Store(&s)
	return nil
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginID,
			Version:          pluginVersion,
			Author:           pluginAuthor,
			GitHubRepository: pluginRepository,
			ConfigFields: []pluginapi.ConfigField{
				{Name: "timezone", Type: pluginapi.ConfigFieldTypeString, Description: "IANA timezone sent to Codex/GPT models, for example Asia/Shanghai."},
				{Name: "models", Type: pluginapi.ConfigFieldTypeArray, Description: "Model patterns to rewrite; '*' is a wildcard. Default: [\"gpt-*\"]."},
				{Name: "rewrite_current_date", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Also rewrite <current_date> to today's date in the target timezone. Default: true."},
				{Name: "inject", Type: pluginapi.ConfigFieldTypeEnum, EnumValues: []string{config.InjectOff, config.InjectIfMissing, config.InjectAlways}, Description: "Append a developer note with the local time: off, if_missing (no <timezone> tag found) or always."},
				{Name: "inject_template", Type: pluginapi.ConfigFieldTypeString, Description: "Injected note template. Placeholders: {tz} {now} {date} {time} {offset} {weekday}."},
			},
		},
		Capabilities: capabilities{RequestNormalizer: true, ManagementAPI: true},
	}
}

func shouldRewrite(s *config.Settings, req pluginapi.RequestTransformRequest) bool {
	return s.Enabled && strings.EqualFold(req.ToFormat, targetFormat) && s.MatchesModel(req.Model)
}

func normalizeRequest(raw []byte) ([]byte, error) {
	var req pluginapi.RequestTransformRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	stats.Seen.Add(1)
	s := current.Load()
	if !shouldRewrite(s, req) {
		return okEnvelope(pluginapi.PayloadResponse{Body: req.Body})
	}
	stats.Matched.Add(1)
	stats.LastModel.Store(req.Model)
	stats.LastAt.Store(nowFunc().Unix())

	res, err := rewrite.Apply(req.Body, *s, nowFunc())
	if err != nil {
		stats.Errors.Add(1)
		return okEnvelope(pluginapi.PayloadResponse{Body: req.Body})
	}
	if res.Changed {
		stats.Rewritten.Add(1)
	}
	if res.Injected {
		stats.Injected.Add(1)
	}
	return okEnvelope(pluginapi.PayloadResponse{Body: res.Body})
}

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}
