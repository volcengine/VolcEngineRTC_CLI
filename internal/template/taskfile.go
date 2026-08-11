// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

// TaskfileName is the per-project task contract file.
const TaskfileName = "vertc.taskfile.yaml"

// Taskfile is the parsed module task contract.
type Taskfile struct {
	Version  int     `yaml:"version"`
	Scene    string  `yaml:"scene"`
	Platform string  `yaml:"platform"`
	SDK      SDK     `yaml:"sdk"`
	Runtime  Runtime `yaml:"runtime,omitempty"`
	Tasks    struct {
		PostCreate []string `yaml:"post_create"`
		Install    []string `yaml:"install"`
		Dev        []string `yaml:"dev"`
	} `yaml:"tasks"`
}

// Runtime describes which generated process owns runtime responsibilities.
// It was added in taskfile v2; zero values intentionally preserve v1 behavior.
type Runtime struct {
	Topology     string `yaml:"topology,omitempty"`
	AgentControl string `yaml:"agent_control,omitempty"`
	Ports        Ports  `yaml:"ports,omitempty"`
}

// Ports declares the local listeners owned by a web-server template.
type Ports struct {
	Web    int `yaml:"web,omitempty"`
	Server int `yaml:"server,omitempty"`
}

// ServerManagedAgent reports whether the companion server, rather than the
// CLI's agent subcommands, owns the conversational-agent lifecycle.
func (t *Taskfile) ServerManagedAgent() bool {
	return t != nil && t.Version >= 2 && t.Runtime.AgentControl == "server"
}

// LoadTaskfile reads the taskfile from a project directory.
func LoadTaskfile(dir string) (*Taskfile, error) {
	path := filepath.Join(dir, TaskfileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.New("vertc.template.not_found", errs.TypeNotFound,
				"%s not found in %q", TaskfileName, dir).
				WithHint("run `vertc init` inside a scaffolded project")
		}
		return nil, errs.Wrap(err, "vertc.cli.internal", "read taskfile: %s", err)
	}
	var tf Taskfile
	if err := yaml.Unmarshal(raw, &tf); err != nil {
		return nil, errs.New("vertc.template.render_failed", errs.TypeIO,
			"parse %s: %s", TaskfileName, err)
	}
	return &tf, nil
}
