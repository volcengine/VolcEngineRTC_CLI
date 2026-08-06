// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errorcodes"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
)

type explainResult struct {
	Query       string `json:"query"`
	Found       bool   `json:"found"`
	Code        string `json:"code,omitempty"`
	Name        string `json:"name,omitempty"`
	Enum        string `json:"enum,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Source      string `json:"source,omitempty"`
	Verified    string `json:"verified,omitempty"`
	Meaning     string `json:"meaning,omitempty"`
	Fix         string `json:"fix,omitempty"`
	DoctorCheck string `json:"doctor_check,omitempty"`
	Hint        string `json:"hint,omitempty"`
}

func (r explainResult) Pretty(w io.Writer) {
	if !r.Found {
		fmt.Fprintf(w, "✗ %s — not in the offline knowledge base\n  %s\n", r.Query, r.Hint)
		return
	}
	header := r.Code
	if r.Enum != "" {
		header = fmt.Sprintf("%s (%s)", r.Code, r.Enum)
	}
	if r.Domain != "" {
		header = fmt.Sprintf("%s [%s]", header, r.Domain)
	}
	fmt.Fprintf(w, "%s\n  %s\n", header, r.Meaning)
	if r.Fix != "" {
		fmt.Fprintf(w, "  fix: %s\n", r.Fix)
	}
	if r.DoctorCheck != "" {
		fmt.Fprintf(w, "  related doctor check: %s → run `%s doctor`\n", r.DoctorCheck, meta.BinName)
	}
	if r.Source != "" {
		fmt.Fprintf(w, "  source: %s\n", r.Source)
	}
	// Do not present an unverified entry as confirmed official fact.
	if r.Verified != "" && r.Verified != errorcodes.VerifiedYes {
		fmt.Fprintf(w, "  ⚠ 未验证（%s）：以官方文档为准\n", r.Verified)
	}
}

func newExplainErrorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain-error <code>",
		Short: "Explain an SDK runtime error code from the offline knowledge base",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			kb, err := errorcodes.LoadWeb()
			if err != nil {
				return errs.Wrap(err, "vertc.cli.internal", "load error-code KB: %s", err)
			}
			query := args[0]
			e, ok := kb.Lookup(query)
			if !ok {
				res := explainResult{
					Query: query, Found: false,
					Hint: fmt.Sprintf("code not found (Web KB has %d entries); run `%s doctor` for general triage", kb.Count(), meta.BinName),
				}
				_ = out().Data(res)
				return errs.New("vertc.explain.unknown_code", errs.TypeNotFound,
					"error code %q is not in the offline knowledge base", query).
					WithHint("run `%s doctor` for general triage", meta.BinName).
					Reported()
			}
			res := explainResult{
				Query: query, Found: true,
				Code: e.Code, Name: e.Name, Enum: e.Enum, Kind: e.Kind,
				Domain: e.Domain, Source: e.Source, Verified: e.Verified,
				Meaning: e.Meaning, Fix: e.Fix, DoctorCheck: e.DoctorCheck,
			}
			if e.DoctorCheck != "" {
				out().Progress("related doctor check: %s", e.DoctorCheck)
			}
			return out().Data(res)
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"An SDK error code appears in logs/console and you need meaning + fix"},
		Avoid:  []string{"CLI's own errors — those carry a vertc.* error.code already"},
		Prereq: []string{"None (knowledge base is embedded, works offline)"},
		Examples: []string{
			meta.BinName + " explain-error INVALID_TOKEN",
			meta.BinName + " explain-error 1202",
		},
	})
	return cmd
}
