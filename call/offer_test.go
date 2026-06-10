// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestOfferBuilderMatchesInboundStructure(t *testing.T) {
	builder := NewOfferBuilder()
	creator, _ := types.ParseJID("5356260450362@lid")
	callerPN, _ := types.ParseJID("559885700260@s.whatsapp.net")
	route, _ := types.ParseJID("5356260450362@lid")

	node, callKey, err := builder.Build(OfferParams{
		CallID:      "006D62BD6943275D0E6D0E35BE40E453",
		CallCreator: creator,
		CallerPN:    callerPN,
		RouteJID:    route,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callKey) != 32 {
		t.Fatalf("expected 32-byte call key, got %d", len(callKey))
	}

	if node.Attrs["call-creator"] != creator {
		t.Fatalf("unexpected call-creator: %v", node.Attrs["call-creator"])
	}
	if node.Attrs["caller_pn"] != callerPN {
		t.Fatalf("unexpected caller_pn: %v", node.Attrs["caller_pn"])
	}
	if fmt.Sprint(node.Attrs["joinable"]) != "1" {
		t.Fatalf("expected joinable=1, got %v", node.Attrs["joinable"])
	}
	if node.Attrs["platform"] != nil {
		t.Fatal("platform must be on call envelope, not offer")
	}

	if _, hasDest := node.GetOptionalChildByTag("destination"); hasDest {
		t.Fatal("offer must not use destination (use single root enc)")
	}

	wantOrder := []string{
		"audio", "audio", "capability", "enc", "encopt", "metadata", "net",
		"uploadfieldstat", "voip_settings",
	}
	children := node.GetChildren()
	if len(children) != len(wantOrder) {
		t.Fatalf("expected %d children, got %d", len(wantOrder), len(children))
	}
	for i, tag := range wantOrder {
		if children[i].Tag != tag {
			t.Fatalf("child[%d]: want %s, got %s", i, tag, children[i].Tag)
		}
	}

	enc := node.GetChildByTag("enc")
	if enc.Attrs["v"] != "2" {
		t.Fatalf("enc v=%v, want 2", enc.Attrs["v"])
	}
	if node.GetChildByTag("metadata").Tag != "metadata" {
		t.Fatal("missing metadata")
	}
	if node.GetChildByTag("uploadfieldstat").Tag != "uploadfieldstat" {
		t.Fatal("missing uploadfieldstat")
	}
}

func TestOfferBuilderWithRTEAndRelay(t *testing.T) {
	builder := NewOfferBuilder()
	creator, _ := types.ParseJID("123@lid")
	route, _ := types.ParseJID("456@lid")
	relay, err := BuildRelayNode(RelayOfferInfo{
		UUID:           "testUUID",
		PeerPID:        "1",
		SelfPID:        "2",
		AttributePadding: true,
		ParticipantJID: creator,
		ParticipantPID: "1",
		Tokens:         map[string][]byte{"0": {0x09, 0x0f}},
		Key:            []byte("dGVzdA=="),
		Endpoints: []RelayEndpoint{{
			RelayName: "fslz10c01", RelayID: "0", TokenID: "0", AuthTokenID: "0",
			C2RRTT: "11", IsFNA: true, IPPort: []byte{0x2d, 0xb4, 0xd9, 0xa2, 0x0d, 0x96},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	node, _, err := builder.Build(OfferParams{
		CallID:      "00ABCDEF1234567890ABCDEF12345678",
		CallCreator: creator,
		RouteJID:    route,
		RTE:         []byte{0x2d, 0xa2, 0xed, 0x98, 0x6b, 0xda},
		Relay:       relay,
	})
	if err != nil {
		t.Fatal(err)
	}
	children := node.GetChildren()
	last := children[len(children)-1]
	if last.Tag != "relay" {
		t.Fatalf("expected relay last child, got %s", last.Tag)
	}
	rteIdx := -1
	for i, c := range children {
		if c.Tag == "rte" {
			rteIdx = i
			break
		}
	}
	if rteIdx < 0 {
		t.Fatal("missing rte child")
	}
	if OfferChildIndex("rte") >= OfferChildIndex("uploadfieldstat") {
		t.Fatal("rte must come before uploadfieldstat")
	}
}

func TestOfferBuilderVideo(t *testing.T) {
	builder := NewOfferBuilder()
	creator, _ := types.ParseJID("123@lid")
	route, _ := types.ParseJID("456@lid")
	node, _, err := builder.Build(OfferParams{
		CallID:      "00ABCDEF1234567890ABCDEF12345678",
		CallCreator: creator,
		RouteJID:    route,
		IsVideo:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.GetChildByTag("video").Tag != "video" {
		t.Fatal("missing video child")
	}
}
