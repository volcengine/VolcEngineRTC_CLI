// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAttachRoundTripsMetadataAndExtendsOwnHelp(t *testing.T) {
	want := Affordance{
		When: []string{"starting"}, Avoid: []string{"stopping"},
		Prereq: []string{"configured"}, Examples: []string{"vertc demo"},
	}
	cmd := &cobra.Command{Use: "demo"}
	var output bytes.Buffer
	cmd.SetOut(&output)
	Attach(cmd, want)

	got, ok := Get(cmd)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() = %+v, %t; want %+v", got, ok, want)
	}
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Affordance:", "When to use:", "Avoid when:", "Prerequisites:", "Examples:", "vertc demo"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("help missing %q:\n%s", text, output.String())
		}
	}
}

func TestGetRejectsMissingOrMalformedAnnotation(t *testing.T) {
	if _, ok := Get(&cobra.Command{}); ok {
		t.Fatal("command without annotations unexpectedly has an affordance")
	}
	cmd := &cobra.Command{Annotations: map[string]string{annotationKey: "malformed\nwhen\tvalid\nunknown\tignored\n"}}
	got, ok := Get(cmd)
	if !ok || !reflect.DeepEqual(got.When, []string{"valid"}) {
		t.Fatalf("decoded malformed annotation = %+v, %t", got, ok)
	}
}
