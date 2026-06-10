// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestPrepareEncryptedNodeDestinationRemovesRootEnc(t *testing.T) {
	peer, _ := types.ParseJID("222:0@lid")
	backend := &mockBackend{
		ownLID: types.JID{User: "111", Server: types.HiddenUserServer},
	}
	bridge := NewSignalingBridge(backend)

	offer := &waBinary.Node{
		Tag: "offer",
		Attrs: waBinary.Attrs{
			"call-id":      "00TEST",
			"call-creator": backend.ownLID,
		},
		Content: []waBinary.Node{
			{Tag: "audio", Attrs: waBinary.Attrs{"enc": "opus", "rate": "16000"}},
			{Tag: "enc", Attrs: waBinary.Attrs{"v": "2", "count": "0"}, Content: []byte{1, 2, 3}},
			{
				Tag: "destination",
				Content: []waBinary.Node{{
					Tag:   "to",
					Attrs: waBinary.Attrs{"jid": peer},
					Content: []waBinary.Node{{
						Tag:     "enc",
						Attrs:   waBinary.Attrs{"v": "2", "count": "0"},
						Content: []byte{9, 9, 9},
					}},
				}},
			},
		},
	}

	prepared, err := bridge.prepareEncryptedNode(context.Background(), offer, peer, []types.JID{peer})
	if err != nil {
		t.Fatal(err)
	}
	if _, hasRootEnc := prepared.GetOptionalChildByTag("enc"); hasRootEnc {
		t.Fatal("root enc should be removed for destination offers")
	}
	dest, ok := prepared.GetOptionalChildByTag("destination")
	if !ok {
		t.Fatal("destination node missing")
	}
	enc := dest.GetChildren()[0].GetChildByTag("enc")
	if enc.Attrs["type"] != "msg" {
		t.Fatalf("expected encrypted destination enc, got attrs %v", enc.Attrs)
	}
}
