// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/output"
)

func TestExplainErrorRecoveryCommandPreservesQuery(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	oldFormat := flagFormat
	t.Cleanup(func() { flagFormat = oldFormat })
	flagFormat = "json"
	for _, query := range []string{"-1000", "NOT_A_REAL_CODE", "$(printf expanded) `printf expanded` '$HOME' \"quoted\"\nline"} {
		t.Run(query, func(t *testing.T) {
			command := newExplainErrorCmd()
			command.SetArgs([]string{"--", query})
			raw := captureCommandStdout(t, func() error {
				if err := command.Execute(); err == nil {
					t.Error("unknown error code succeeded")
				}
				return nil
			})
			var envelope struct {
				Data explainResult `json:"data"`
			}
			if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
				t.Fatal(err)
			}
			_, hint, ok := strings.Cut(envelope.Data.Hint, "search official docs with `")
			if !ok {
				t.Fatalf("missing search command: %s", envelope.Data.Hint)
			}
			retry, ok := strings.CutSuffix(hint, "`; run `"+meta.BinName+" doctor` for general triage")
			if !ok {
				t.Fatalf("unexpected hint: %s", hint)
			}
			parsed, err := exec.Command(shell, "-c", "set -- "+retry+"; printf '%s\\0' \"$@\"").Output()
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSuffix(string(parsed), "\x00"), "\x00")
			if len(args) != 5 || args[0] != meta.BinName || args[1] != "docs" || args[4] != query {
				t.Fatalf("recovery command arguments = %q", args)
			}
			fake := &fakeTopicDocsClient{}
			if _, _, err := runDocsCommand(t, fake, output.FormatJSON, args[2:]...); err != nil {
				t.Fatal(err)
			}
			if fake.searchQuery != query || fake.closed != 1 {
				t.Fatalf("search query = %q, closed = %d", fake.searchQuery, fake.closed)
			}
		})
	}
}
