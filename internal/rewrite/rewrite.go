// Package rewrite changes the timezone and date a Codex Responses request
// tells the model about.
//
// Codex CLI describes the client environment with a block like:
//
//	<environment_context>
//	  <current_date>2026-02-26</current_date>
//	  <timezone>America/Los_Angeles</timezone>
//	</environment_context>
//
// The block lives inside input[].content[].text (and occasionally in
// instructions). Rewrite replaces both tags with values computed in the
// configured timezone, and can append a developer message for clients that do
// not send an environment context at all.
package rewrite

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"cpa-codex-timezone/internal/config"
)

var (
	// Only tags inside an environment_context block are rewritten, so the same
	// tag names quoted in user prompts or pasted files reach the model intact.
	envContextBlock = regexp.MustCompile(`(?s)<environment_context(?:\s[^>]*)?>.*?</environment_context>`)
	timezoneTag     = regexp.MustCompile(`<timezone(?:\s[^>]*)?(?:/>|>[^<]*</timezone>)`)
	currentDateTag  = regexp.MustCompile(`<current_date(?:\s[^>]*)?(?:/>|>[^<]*</current_date>)`)
)

// Result reports what Apply changed.
type Result struct {
	Body             []byte `json:"-"`
	Changed          bool   `json:"changed"`
	TimezoneReplaced int    `json:"timezone_replaced"`
	DateReplaced     int    `json:"date_replaced"`
	Injected         bool   `json:"injected"`
	InjectedText     string `json:"injected_text,omitempty"`
}

// Apply rewrites body according to s, using now as the current instant. The
// original body is returned unchanged when it is not a JSON object.
func Apply(body []byte, s config.Settings, now time.Time) (Result, error) {
	res := Result{Body: body}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return res, fmt.Errorf("request body is not a JSON object")
	}
	local := now.In(s.Location)
	out := body

	rewriteText := func(path, text string) error {
		updated, tz, date := rewriteString(text, s, local)
		res.TimezoneReplaced += tz
		res.DateReplaced += date
		if updated == text {
			return nil
		}
		var err error
		out, err = sjson.SetBytes(out, path, updated)
		return err
	}

	if instr := gjson.GetBytes(out, "instructions"); instr.Type == gjson.String {
		if err := rewriteText("instructions", instr.String()); err != nil {
			return res, err
		}
	}

	input := gjson.GetBytes(out, "input")
	switch {
	case input.Type == gjson.String:
		if err := rewriteText("input", input.String()); err != nil {
			return res, err
		}
	case input.IsArray():
		for i, item := range input.Array() {
			if strings.EqualFold(item.Get("role").String(), "assistant") {
				continue
			}
			content := item.Get("content")
			if content.Type == gjson.String {
				if err := rewriteText(fmt.Sprintf("input.%d.content", i), content.String()); err != nil {
					return res, err
				}
				continue
			}
			if !content.IsArray() {
				continue
			}
			for j, part := range content.Array() {
				if text := part.Get("text"); text.Type == gjson.String {
					if err := rewriteText(fmt.Sprintf("input.%d.content.%d.text", i, j), text.String()); err != nil {
						return res, err
					}
				}
			}
		}
	}

	if shouldInject(s.Inject, res.TimezoneReplaced) {
		text := RenderTemplate(s.InjectTemplate, s.Timezone, local)
		var err error
		out, err = appendDeveloperMessage(out, text)
		if err != nil {
			return res, err
		}
		res.Injected = true
		res.InjectedText = text
	}

	res.Body = out
	res.Changed = string(out) != string(body)
	return res, nil
}

func shouldInject(mode string, timezoneTags int) bool {
	switch mode {
	case config.InjectAlways:
		return true
	case config.InjectIfMissing:
		return timezoneTags == 0
	default:
		return false
	}
}

func rewriteString(text string, s config.Settings, local time.Time) (string, int, int) {
	tzCount, dateCount := 0, 0
	if !strings.Contains(text, "<environment_context") {
		return text, 0, 0
	}
	text = envContextBlock.ReplaceAllStringFunc(text, func(block string) string {
		block = timezoneTag.ReplaceAllStringFunc(block, func(string) string {
			tzCount++
			return "<timezone>" + s.Timezone + "</timezone>"
		})
		if s.RewriteCurrentDate {
			block = currentDateTag.ReplaceAllStringFunc(block, func(string) string {
				dateCount++
				return "<current_date>" + local.Format("2006-01-02") + "</current_date>"
			})
		}
		return block
	})
	return text, tzCount, dateCount
}

// RenderTemplate expands {tz}, {now}, {date}, {time}, {offset} and {weekday}.
func RenderTemplate(tpl, tz string, local time.Time) string {
	return strings.NewReplacer(
		"{tz}", tz,
		"{now}", local.Format("2006-01-02 15:04:05 -07:00"),
		"{date}", local.Format("2006-01-02"),
		"{time}", local.Format("15:04:05"),
		"{offset}", local.Format("-07:00"),
		"{weekday}", local.Weekday().String(),
	).Replace(tpl)
}

// appendDeveloperMessage adds the note at the end of input so the stable
// request prefix, and therefore upstream prompt caching, is preserved.
func appendDeveloperMessage(body []byte, text string) ([]byte, error) {
	msg := map[string]any{
		"type": "message",
		"role": "developer",
		"content": []map[string]string{{
			"type": "input_text",
			"text": text,
		}},
	}
	input := gjson.GetBytes(body, "input")
	switch {
	case input.IsArray():
		return sjson.SetBytes(body, "input.-1", msg)
	case input.Type == gjson.String:
		items := []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []map[string]string{{"type": "input_text", "text": input.String()}},
			},
			msg,
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		return sjson.SetRawBytes(body, "input", raw)
	default:
		return sjson.SetBytes(body, "input", []any{msg})
	}
}
