// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package product

import (
	"reflect"
	"testing"
)

func TestCurrentReturnsSoleDefaultProfile(t *testing.T) {
	previous := SelectedID
	SelectedID = DefaultID
	t.Cleanup(func() { SelectedID = previous })
	profile := Current()
	if profile != defaultProfile {
		t.Fatalf("Current() = %+v, want %+v", profile, defaultProfile)
	}
	if ids := RegisteredIDs(); !reflect.DeepEqual(ids, []string{DefaultID}) {
		t.Fatalf("registered IDs = %v", ids)
	}
}

func TestCurrentFailsClosedForUnknownSelection(t *testing.T) {
	previous := SelectedID
	SelectedID = "unregistered"
	t.Cleanup(func() { SelectedID = previous })
	defer func() {
		if recover() == nil {
			t.Fatal("Current accepted an unknown profile")
		}
	}()
	_ = Current()
}

func TestProfileIsReturnedByValue(t *testing.T) {
	first := Current()
	first.DisplayName = "changed"
	if second := Current(); second.DisplayName != defaultProfile.DisplayName {
		t.Fatalf("profile mutation escaped: %+v", second)
	}
}

func TestValidateRejectsIncompleteAndUnsafeProfiles(t *testing.T) {
	missing := defaultProfile
	missing.OpenAPIEndpoint = ""
	if err := validate(missing); err == nil {
		t.Fatal("validate accepted missing endpoint")
	}
	remote := defaultProfile
	remote.OAuthRemoteRedirectURI = ""
	if err := validate(remote); err == nil {
		t.Fatal("validate accepted incomplete remote OAuth")
	}
}
