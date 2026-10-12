// Package config parses and validates the plugin-owned configuration block
// found under plugins.configs.codex-timezone in the CPA config file.
package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	// Hosts without system zoneinfo (notably Windows) still resolve IANA names.
	_ "time/tzdata"

	"gopkg.in/yaml.v3"
)

const (
	InjectOff       = "off"
	InjectIfMissing = "if_missing"
	InjectAlways    = "always"

	DefaultTimezone       = "Asia/Shanghai"
	DefaultInjectTemplate = "Current timezone: {tz}. Current local time: {now}."
)

var DefaultModels = []string{"gpt-*"}

// File mirrors the YAML/JSON shape users write. Pointer fields distinguish
// "unset" from an explicit zero value so defaults can be applied.
type File struct {
	Enabled            *bool    `yaml:"enabled" json:"enabled"`
	Priority           int      `yaml:"priority" json:"priority"`
	Timezone           string   `yaml:"timezone" json:"timezone"`
	Models             []string `yaml:"models" json:"models"`
	RewriteCurrentDate *bool    `yaml:"rewrite_current_date" json:"rewrite_current_date"`
	Inject             string   `yaml:"inject" json:"inject"`
	InjectTemplate     string   `yaml:"inject_template" json:"inject_template"`
}

// Settings is the validated, ready-to-use configuration.
type Settings struct {
	Enabled            bool           `json:"enabled"`
	Timezone           string         `json:"timezone"`
	Location           *time.Location `json:"-"`
	Models             []string       `json:"models"`
	RewriteCurrentDate bool           `json:"rewrite_current_date"`
	Inject             string         `json:"inject"`
	InjectTemplate     string         `json:"inject_template"`

	modelPatterns []*regexp.Regexp
}

// Default returns the settings used before CPA sends any configuration.
func Default() Settings {
	s, _ := FromFile(File{})
	return s
}

// ParseYAML decodes the config_yaml payload CPA sends on plugin.register and
// plugin.reconfigure.
func ParseYAML(raw []byte) (Settings, error) {
	var f File
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := yaml.Unmarshal(raw, &f); err != nil {
			return Settings{}, fmt.Errorf("invalid plugin config: %w", err)
		}
	}
	return FromFile(f)
}

// FromFile applies defaults and validates every field.
func FromFile(f File) (Settings, error) {
	s := Settings{
		Enabled:            f.Enabled != nil && *f.Enabled,
		Timezone:           strings.TrimSpace(f.Timezone),
		RewriteCurrentDate: f.RewriteCurrentDate == nil || *f.RewriteCurrentDate,
		Inject:             strings.ToLower(strings.TrimSpace(f.Inject)),
		InjectTemplate:     f.InjectTemplate,
	}
	if s.Timezone == "" {
		s.Timezone = DefaultTimezone
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return Settings{}, fmt.Errorf("unknown timezone %q", s.Timezone)
	}
	s.Location = loc

	switch s.Inject {
	case "":
		s.Inject = InjectIfMissing
	case InjectOff, InjectIfMissing, InjectAlways:
	default:
		return Settings{}, fmt.Errorf("inject must be one of off, if_missing, always (got %q)", f.Inject)
	}
	if strings.TrimSpace(s.InjectTemplate) == "" {
		s.InjectTemplate = DefaultInjectTemplate
	}

	for _, m := range f.Models {
		if m = strings.TrimSpace(m); m != "" {
			s.Models = append(s.Models, m)
		}
	}
	if len(s.Models) == 0 {
		s.Models = append([]string(nil), DefaultModels...)
	}
	for _, m := range s.Models {
		s.modelPatterns = append(s.modelPatterns, globToRegexp(m))
	}
	return s, nil
}

// MatchesModel reports whether model matches any configured pattern. Patterns
// are case-insensitive and support '*' as a wildcard; a pattern without '*'
// must match exactly.
func (s Settings) MatchesModel(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, re := range s.modelPatterns {
		if re.MatchString(model) {
			return true
		}
	}
	return false
}

func globToRegexp(pattern string) *regexp.Regexp {
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile("(?i)^" + strings.Join(parts, ".*") + "$")
}
