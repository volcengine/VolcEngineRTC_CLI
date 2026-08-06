// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package meta

import "testing"

func TestBusinessIDIsStableAcrossBinaryAliases(t *testing.T) {
	previous := BinName
	BinName = "volc-aiav"
	t.Cleanup(func() { BinName = previous })

	if BusinessID != "vertc_cli" {
		t.Fatalf("BusinessID = %q, want vertc_cli", BusinessID)
	}
}
