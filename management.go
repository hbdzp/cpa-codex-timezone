package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"

	"cpa-codex-timezone/internal/config"
	"cpa-codex-timezone/internal/rewrite"
	"cpa-codex-timezone/internal/ui"
)

const (
	resourceUIPath = "/v0/resource/plugins/" + pluginID + "/ui"
	statusPath     = "/v0/management/" + pluginID + "/status"
	previewPath    = "/v0/management/" + pluginID + "/preview"
)

// Handlers are attached by the host, so the wire format only carries the
// declarative fields of pluginapi.ManagementRoute / ResourceRoute.
type routeDecl struct {
	Method      string `json:"Method,omitempty"`
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type managementRegistrationResponse struct {
	Routes    []routeDecl `json:"routes"`
	Resources []routeDecl `json:"resources"`
}

func managementRegistration() managementRegistrationResponse {
	return managementRegistrationResponse{
		Routes: []routeDecl{
			{Method: http.MethodGet, Path: statusPath, Description: "Effective timezone settings and rewrite counters."},
			{Method: http.MethodPost, Path: previewPath, Description: "Dry-run the timezone rewrite on a sample Codex request."},
		},
		Resources: []routeDecl{
			{Path: "/ui", Menu: "时区设置", Description: "Configure the timezone CPA reports to Codex/GPT models."},
		},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	method := strings.ToUpper(req.Method)
	switch {
	case req.Path == resourceUIPath && method == http.MethodGet:
		return okEnvelope(pluginapi.ManagementResponse{
			StatusCode: http.StatusOK,
			Headers: http.Header{
				"Content-Type":  {"text/html; charset=utf-8"},
				"Cache-Control": {"no-store"},
			},
			Body: ui.IndexHTML,
		})
	case req.Path == statusPath && method == http.MethodGet:
		return jsonResponse(http.StatusOK, statusPayload())
	case req.Path == previewPath && method == http.MethodPost:
		return handlePreview(req.Body)
	default:
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

type statusResponse struct {
	Plugin   string          `json:"plugin"`
	Version  string          `json:"version"`
	Settings config.Settings `json:"settings"`
	LocalNow string          `json:"local_now"`
	Offset   string          `json:"offset"`
	Stats    statsSnapshot   `json:"stats"`
}

type statsSnapshot struct {
	Seen      int64  `json:"seen"`
	Matched   int64  `json:"matched"`
	Rewritten int64  `json:"rewritten"`
	Injected  int64  `json:"injected"`
	Errors    int64  `json:"errors"`
	LastModel string `json:"last_model,omitempty"`
	LastAt    string `json:"last_at,omitempty"`
}

func statusPayload() statusResponse {
	s := current.Load()
	now := nowFunc().In(s.Location)
	snap := statsSnapshot{
		Seen:      stats.Seen.Load(),
		Matched:   stats.Matched.Load(),
		Rewritten: stats.Rewritten.Load(),
		Injected:  stats.Injected.Load(),
		Errors:    stats.Errors.Load(),
	}
	if m, ok := stats.LastModel.Load().(string); ok {
		snap.LastModel = m
	}
	if at := stats.LastAt.Load(); at > 0 {
		snap.LastAt = time.Unix(at, 0).In(s.Location).Format(time.RFC3339)
	}
	return statusResponse{
		Plugin:   pluginID,
		Version:  pluginVersion,
		Settings: *s,
		LocalNow: now.Format("2006-01-02 15:04:05"),
		Offset:   now.Format("-07:00"),
		Stats:    snap,
	}
}

type previewRequest struct {
	Model    string          `json:"model"`
	Body     json.RawMessage `json:"body"`
	Settings *config.File    `json:"settings,omitempty"`
}

type previewResponse struct {
	Matched bool            `json:"matched"`
	Result  rewrite.Result  `json:"result"`
	Body    json.RawMessage `json:"body"`
}

func handlePreview(raw []byte) ([]byte, error) {
	var req previewRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
	}
	s := *current.Load()
	if req.Settings != nil {
		enabled := true
		req.Settings.Enabled = &enabled
		parsed, err := config.FromFile(*req.Settings)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		s = parsed
	}
	body := []byte(req.Body)
	var asString string
	if json.Unmarshal(body, &asString) == nil {
		body = []byte(asString)
	}
	if !json.Valid(body) {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "请求体不是合法的 JSON"})
	}
	model := req.Model
	if model == "" {
		var probe struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &probe)
		model = probe.Model
	}
	matched := s.MatchesModel(model)
	resp := previewResponse{Matched: matched, Body: body}
	if !matched {
		return jsonResponse(http.StatusOK, resp)
	}
	res, err := rewrite.Apply(body, s, nowFunc())
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	resp.Result = res
	resp.Body = res.Body
	return jsonResponse(http.StatusOK, resp)
}

func jsonResponse(status int, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return okEnvelope(pluginapi.ManagementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body:       body,
	})
}
