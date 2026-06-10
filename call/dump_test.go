// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"encoding/json"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
)

func TestDumpNode(t *testing.T) {
	node := &waBinary.Node{
		Tag: "offer",
		Attrs: waBinary.Attrs{
			"call-id": "00TEST",
			"media":   "audio",
		},
		Content: []waBinary.Node{{
			Tag:   "enc",
			Attrs: waBinary.Attrs{"v": "2"},
			Content: []byte{1, 2, 3},
		}},
	}
	data, err := DumpNode(node)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err = json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["tag"] != "offer" {
		t.Fatalf("unexpected tag: %v", parsed["tag"])
	}
}
