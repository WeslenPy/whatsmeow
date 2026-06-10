// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestBuildRelayNode(t *testing.T) {
	creator, _ := types.ParseJID("5356260450362@lid")
	node, err := BuildRelayNode(RelayOfferInfo{
		UUID:             "LKj6Tn8J1CcuhldV",
		PeerPID:          "1",
		SelfPID:          "2",
		AttributePadding: true,
		ParticipantJID:   creator,
		ParticipantPID:   "1",
		Tokens: map[string][]byte{
			"0": {0x09, 0x0f, 0x01},
		},
		AuthTokens: map[string][]byte{
			"0": {0x09, 0x03, 0x18},
		},
		Key:    []byte("qZ86qnr33y9e/sEBJ0bOVA=="),
		HBHKey: []byte("1+bITNjPoH4CmtJ4S8WCMwXhiPoT8LMwHi2VcNJJ"),
		Endpoints: []RelayEndpoint{{
			RelayID: "0", RelayName: "fslz10c01", TokenID: "0", AuthTokenID: "0",
			C2RRTT: "11", IsFNA: true,
			IPPort: []byte{0x2d, 0xb4, 0xd9, 0xa2, 0x0d, 0x96},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Tag != "relay" {
		t.Fatalf("expected relay tag, got %s", node.Tag)
	}
	if node.Attrs["uuid"] != "LKj6Tn8J1CcuhldV" {
		t.Fatalf("unexpected uuid: %v", node.Attrs["uuid"])
	}
	if node.GetChildByTag("hbh_key").Content == nil {
		t.Fatal("missing hbh_key")
	}
}

func TestDecodeRelayKey(t *testing.T) {
	key := []byte("qZ86qnr33y9e/sEBJ0bOVA==")
	decoded, err := DecodeRelayKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 16 {
		t.Fatalf("expected 16-byte key, got %d", len(decoded))
	}
}
