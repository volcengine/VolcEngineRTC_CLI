// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

// envRefPattern matches a `${ENV_VAR}` placeholder.
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Path returns the config path for a project directory ("" = cwd).
func Path(dir string) string {
	return filepath.Join(dir, meta.ConfigFileName)
}

// Find locates the config by walking up from dir until it finds one, returning
// its path. It stops at the filesystem root.
func Find(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", errs.Wrap(err, "vertc.cli.internal", "resolve path: %s", err)
	}
	for {
		p := Path(abs)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", errs.New("vertc.config.not_found", errs.TypeNotFound,
				"%s not found in %q or any parent", meta.ConfigFileName, dir).
				WithHint("run `%s init --scene voice-agent --platform web` to scaffold one", meta.BinName)
		}
		abs = parent
	}
}

// Load reads and parses the config at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.New("vertc.config.not_found", errs.TypeNotFound,
				"%s not found", path).
				WithHint("run `%s init` to scaffold one", meta.BinName)
		}
		return nil, errs.Wrap(err, "vertc.cli.internal", "read config: %s", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, errs.New("vertc.config.parse_error", errs.TypeValidation,
			"failed to parse %s: %s", path, err).
			WithHint("check YAML syntax")
	}
	Normalize(&c)
	return &c, nil
}

// LoadNearest finds and loads the config starting from dir.
func LoadNearest(dir string) (*Config, string, error) {
	path, err := Find(dir)
	if err != nil {
		return nil, "", err
	}
	c, err := Load(path)
	return c, path, err
}

// Marshal serializes the normalized project configuration without writing it.
func Marshal(c *Config) ([]byte, error) {
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	Normalize(c)
	out, err := yaml.Marshal(c)
	if err != nil {
		return nil, errs.Wrap(err, "vertc.cli.internal", "marshal config: %s", err)
	}
	return out, nil
}

// Save writes the config to path (0644), creating parent dirs as needed.
func Save(c *Config, path string) error {
	out, err := Marshal(c)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errs.New("vertc.config.write_failed", errs.TypeIO,
				"create dir %q: %s", dir, err)
		}
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return errs.New("vertc.config.write_failed", errs.TypeIO,
			"write %q: %s", path, err)
	}
	return nil
}

// Normalize applies schema defaults for newly added optional sections.
func Normalize(c *Config) {
	if c.OpenAPI.Endpoint == "" {
		c.OpenAPI.Endpoint = DefaultOpenAPIEndpoint
	}
	if c.OpenAPI.Service == "" {
		c.OpenAPI.Service = DefaultOpenAPIService
	}
	if c.OpenAPI.Region == "" {
		c.OpenAPI.Region = DefaultOpenAPIRegion
	}
}

// ResolveEnv expands `${ENV}` placeholders in s using the environment.
// Returns the resolved value and the list of variable names that were missing.
func ResolveEnv(s string) (resolved string, missing []string) {
	resolved = envRefPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := envRefPattern.FindStringSubmatch(m)[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		missing = append(missing, name)
		return m // leave placeholder in place
	})
	return resolved, missing
}
