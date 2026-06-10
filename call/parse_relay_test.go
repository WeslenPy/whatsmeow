// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestParseRelayNode(t *testing.T) {
	creator, _ := types.ParseJID("5356260450362@lid")
	node, err := BuildRelayNode(RelayOfferInfo{
		UUID:           "LKj6Tn8J1CcuhldV",
		PeerPID:        "1",
		SelfPID:        "2",
		ParticipantJID: creator,
		Tokens:         map[string][]byte{"0": {0x09, 0x0f, 0x01}},
		AuthTokens:     map[string][]byte{"0": {0x09, 0x03, 0x18}},
		Key:            []byte("qZ86qnr33y9e/sEBJ0bOVA=="),
		HBHKey:         []byte("1+bITNjPoH4CmtJ4S8WCMwXhiPoT8LMwHi2VcNJJ"),
		Endpoints: []RelayEndpoint{{
			RelayID: "0", RelayName: "fslz10c01", TokenID: "0", AuthTokenID: "0",
			IPPort: []byte{0x2d, 0xb4, 0xd9, 0xa2, 0x0d, 0x96},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseRelayNode(node)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.UUID != "LKj6Tn8J1CcuhldV" {
		t.Fatalf("uuid mismatch: %s", parsed.UUID)
	}
	if len(parsed.Endpoints) != 1 || parsed.Endpoints[0].RelayName != "fslz10c01" {
		t.Fatalf("unexpected endpoints: %+v", parsed.Endpoints)
	}
	if parsed.Tokens["0"] == nil {
		t.Fatal("missing token 0")
	}
	if string(parsed.HBHKeyRaw) != "1+bITNjPoH4CmtJ4S8WCMwXhiPoT8LMwHi2VcNJJ" {
		t.Fatalf("hbh_key mismatch: %q", parsed.HBHKeyRaw)
	}
	_ = waBinary.Node{}
}
