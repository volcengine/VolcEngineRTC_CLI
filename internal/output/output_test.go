// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
)

func newTestWriter(f Format) (*Writer, *bytes.Buffer, *bytes.Buffer) {
	var o, e bytes.Buffer
	return &Writer{Out: &o, Err: &e, Format: f}, &o, &e
}

func TestDataJSONEnvelope(t *testing.T) {
	SetNoticeProvider(nil)
	w, o, e := newTestWriter(FormatJSON)
	w.Progress("progress goes to stderr")
	if err := w.Data(map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, o.String())
	}
	if !env.OK {
		t.Fatal("expected ok=true")
	}
	// stdout must not contain the progress line.
	if strings.Contains(o.String(), "progress goes to stderr") {
		t.Fatal("progress leaked into stdout")
	}
	if !strings.Contains(e.String(), "progress goes to stderr") {
		t.Fatal("progress missing from stderr")
	}
}

func TestJSONEnvelopeNotice(t *testing.T) {
	SetNoticeProvider(func() map[string]any {
		return map[string]any{"update": map[string]string{"latest": "1.2.3"}}
	})
	t.Cleanup(func() { SetNoticeProvider(nil) })
	w, o, _ := newTestWriter(FormatJSON)
	if err := w.Data(map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Notice["update"] == nil {
		t.Fatalf("missing update notice: %+v", env)
	}
}

func TestFailJSONEnvelopeNotice(t *testing.T) {
	SetNoticeProvider(func() map[string]any { return map[string]any{"skills": "stale"} })
	t.Cleanup(func() { SetNoticeProvider(nil) })
	w, o, _ := newTestWriter(FormatJSON)
	w.Fail(errs.New("vertc.cli.internal", errs.TypeInternal, "boom"))
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Notice["skills"] == nil || env.Error == nil {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

func TestFailEnvelopeAndExit(t *testing.T) {
	w, o, _ := newTestWriter(FormatJSON)
	code := w.Fail(errs.New("vertc.config.missing_field", errs.TypeValidation, "boom").
		WithParam("rtc.app_id").WithHint("fix it"))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != "vertc.config.missing_field" {
		t.Fatalf("unexpected error envelope: %+v", env)
	}
}

func TestFailEnvelopeIncludesStructuredDetails(t *testing.T) {
	w, o, _ := newTestWriter(FormatJSON)
	w.Fail(errs.New("vertc.dev.selection_required", errs.TypePrecondition, "choose").
		WithDetails(map[string]any{"apps": []map[string]string{{"app_id": "public"}}}))
	var env Envelope
	if err := json.Unmarshal(o.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Details["apps"] == nil {
		t.Fatalf("missing details: %+v", env)
	}
}

func TestFailReportedSkipsStdout(t *testing.T) {
	w, o, e := newTestWriter(FormatJSON)
	err := errs.New("vertc.doctor.failed", errs.TypePrecondition, "1 failed").
		WithHint("run doctor").Reported()
	code := w.Fail(err)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if o.Len() != 0 {
		t.Fatalf("reported error should not write stdout, got %q", o.String())
	}
	if !strings.Contains(e.String(), "run doctor") {
		t.Fatal("hint should still go to stderr")
	}
}

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"json", "pretty", "table"} {
		if _, err := ParseFormat(ok); err != nil {
			t.Fatalf("ParseFormat(%q): %v", ok, err)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Fatal("expected error for xml")
	}
}
