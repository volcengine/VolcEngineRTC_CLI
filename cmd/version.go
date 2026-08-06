// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

type versionInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildDate     string `json:"build_date"`
	ReleaseMarker string `json:"-"`
}

func (v versionInfo) Pretty(w io.Writer) {
	fmt.Fprintf(w, "%s %s (commit %s, built %s)\n", v.Name, v.Version, v.Commit, v.BuildDate)
}

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print CLI version information",
		RunE: func(c *cobra.Command, args []string) error {
			return out().Data(versionInfo{
				Name:          meta.BinName,
				Version:       meta.Version,
				Commit:        meta.Commit,
				BuildDate:     meta.BuildDate,
				ReleaseMarker: meta.ReleaseMarker,
			})
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Reporting or debugging which CLI build is in use"},
		Examples: []string{meta.BinName + " version", meta.BinName + " version --format pretty"},
	})
	return cmd
}
