// Package config provides the toolkit configuration model: a typed mirror of
// PSAppDeployToolkit's config.psd1 sections (Assets, MSI, Toolkit, UI) with
// embedded defaults and overlay-merge semantics.
//
// Defaults are generated from PSADT's ImportsLast.ps1 by tools/psd1convert and
// embedded as JSON (valid YAML). Package overlays are YAML files using the
// same PascalCase key names as PSADT's config.psd1, so existing PSADT config
// files translate line-for-line.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed embedded/config.default.json
var defaultConfig []byte

// Config mirrors the section structure of PSADT's config.psd1.
type Config struct {
	Assets  Assets  `yaml:"Assets"`
	MSI     MSI     `yaml:"MSI"`
	Toolkit Toolkit `yaml:"Toolkit"`
	UI      UI      `yaml:"UI"`
}

// Assets mirrors config.psd1 [Assets]: filenames or Base64 strings.
type Assets struct {
	Logo        string `yaml:"Logo"`
	LogoDark    string `yaml:"LogoDark"`
	Banner      string `yaml:"Banner"`
	TaskbarIcon string `yaml:"TaskbarIcon"`
}

// MSI mirrors config.psd1 [MSI].
type MSI struct {
	InstallParams        string `yaml:"InstallParams"`
	LoggingOptions       string `yaml:"LoggingOptions"`
	LogPath              string `yaml:"LogPath"`
	LogPathNoAdminRights string `yaml:"LogPathNoAdminRights"`
	MutexWaitTime        int    `yaml:"MutexWaitTime"` // seconds
	SilentParams         string `yaml:"SilentParams"`
	UninstallParams      string `yaml:"UninstallParams"`
}

// Toolkit mirrors config.psd1 [Toolkit].
type Toolkit struct {
	CachePath                 string `yaml:"CachePath"`
	CompanyName               string `yaml:"CompanyName"`
	CompressLogs              bool   `yaml:"CompressLogs"`
	FileCopyMode              string `yaml:"FileCopyMode"` // Native | Robocopy
	LogAppend                 bool   `yaml:"LogAppend"`
	LogDebugMessage           bool   `yaml:"LogDebugMessage"`
	LogMaxHierarchy           int    `yaml:"LogMaxHierarchy"`
	LogMaxHistory             int    `yaml:"LogMaxHistory"`
	LogMaxSize                int    `yaml:"LogMaxSize"` // megabytes
	LogPath                   string `yaml:"LogPath"`
	LogPathNoAdminRights      string `yaml:"LogPathNoAdminRights"`
	LogToHierarchy            bool   `yaml:"LogToHierarchy"`
	LogToSubfolder            bool   `yaml:"LogToSubfolder"`
	LogStyle                  string `yaml:"LogStyle"` // CMTrace | Legacy
	LogWriteToHost            bool   `yaml:"LogWriteToHost"`
	LogHostOutputToStdStreams bool   `yaml:"LogHostOutputToStdStreams"`
	RegPath                   string `yaml:"RegPath"`
	RegPathNoAdminRights      string `yaml:"RegPathNoAdminRights"`
	TempPath                  string `yaml:"TempPath"`
	TempPathNoAdminRights     string `yaml:"TempPathNoAdminRights"`
}

// UI mirrors config.psd1 [UI].
type UI struct {
	BalloonNotifications         bool   `yaml:"BalloonNotifications"`
	DialogStyle                  string `yaml:"DialogStyle"` // Fluent | Classic
	FluentAccentColor            uint32 `yaml:"FluentAccentColor"`
	FluentAccentColorDark        uint32 `yaml:"FluentAccentColorDark"`
	DefaultExitCode              int    `yaml:"DefaultExitCode"`              // dialog timeout, default 1618
	DefaultPromptPersistInterval int    `yaml:"DefaultPromptPersistInterval"` // seconds
	DefaultTimeout               int    `yaml:"DefaultTimeout"`               // seconds
	DeferExitCode                int    `yaml:"DeferExitCode"`                // default 1602
	LanguageOverride             string `yaml:"LanguageOverride"`
	PromptToSaveTimeout          int    `yaml:"PromptToSaveTimeout"`          // seconds
	RestartPromptPersistInterval int    `yaml:"RestartPromptPersistInterval"` // seconds
}

// Default returns the embedded PSADT default configuration.
func Default() (*Config, error) {
	c := &Config{}
	if err := yaml.Unmarshal(defaultConfig, c); err != nil {
		return nil, fmt.Errorf("config: parsing embedded defaults: %w", err)
	}
	return c, nil
}

// Load returns the defaults with the YAML overlay file at path merged on top.
// An empty path returns the defaults untouched. Overlay semantics follow
// yaml.v3 decoding into a populated struct: only keys present in the document
// are modified; absent keys keep their default values.
func Load(path string) (*Config, error) {
	c, err := Default()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return c, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: reading overlay: %w", err)
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("config: parsing overlay %s: %w", path, err)
	}
	return c, nil
}

var envToken = regexp.MustCompile(`(?i)\$env:([A-Za-z_][A-Za-z0-9_()]*)`)

// ExpandEnv expands PowerShell-style `$env:Name` tokens using the process
// environment, mirroring how PSADT expands path strings from config.psd1
// (e.g. `$env:ProgramData\Logs\Software`).
func ExpandEnv(s string) string {
	return envToken.ReplaceAllStringFunc(s, func(m string) string {
		name := m[strings.Index(m, ":")+1:]
		return os.Getenv(name)
	})
}

// ExpandPaths applies ExpandEnv to every path-bearing field in the config.
// Call once after Load, before use.
func (c *Config) ExpandPaths() {
	for _, p := range []*string{
		&c.Toolkit.CachePath, &c.Toolkit.LogPath, &c.Toolkit.LogPathNoAdminRights,
		&c.Toolkit.TempPath, &c.Toolkit.TempPathNoAdminRights,
		&c.MSI.LogPath, &c.MSI.LogPathNoAdminRights,
	} {
		*p = ExpandEnv(*p)
	}
}

// Lookup resolves a `Section\Key` reference (as used by string-table
// interpolations like `{Toolkit\CompanyName}`) to its string value.
func (c *Config) Lookup(ref string) (string, bool) {
	section, key, ok := strings.Cut(ref, `\`)
	if !ok {
		return "", false
	}
	var sec any
	switch section {
	case "Assets":
		sec = c.Assets
	case "MSI":
		sec = c.MSI
	case "Toolkit":
		sec = c.Toolkit
	case "UI":
		sec = c.UI
	default:
		return "", false
	}
	// Round-trip through YAML to honor the PascalCase key names without
	// reflection here; sections are tiny so cost is negligible.
	raw, err := yaml.Marshal(sec)
	if err != nil {
		return "", false
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return "", false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	return fmt.Sprintf("%v", v), true
}
