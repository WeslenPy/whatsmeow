// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"strings"
	"testing"
)

func TestGenerateCallID(t *testing.T) {
	id, err := GenerateCallID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "00") {
		t.Fatalf("call id should start with 00, got %q", id)
	}
	if len(id) != 32 {
		t.Fatalf("call id should be 32 chars, got %d: %q", len(id), id)
	}
	for _, c := range id[2:] {
		if c < '0' || (c > '9' && c < 'A') || c > 'F' {
			t.Fatalf("call id should be uppercase hex after prefix, got %q", id)
		}
	}
}
