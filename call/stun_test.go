// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestBuildSTUNMessage(t *testing.T) {
	txID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	msg := buildSTUNMessage(stunBindingRequest, txID, []stunAttr{{Type: stunAttrWAToken, Value: []byte{0x09, 0x0f}}})
	if len(msg) < 24 {
		t.Fatalf("message too short: %d", len(msg))
	}
	if binary.BigEndian.Uint16(msg[0:2]) != stunBindingRequest {
		t.Fatalf("unexpected type %x", binary.BigEndian.Uint16(msg[0:2]))
	}
	if binary.BigEndian.Uint32(msg[4:8]) != stunMagicCookie {
		t.Fatalf("unexpected cookie %x", binary.BigEndian.Uint32(msg[4:8]))
	}
}

func TestRelayLatencyTE(t *testing.T) {
	results := []*STUNResult{{
		RelayName:  "fslz10c01",
		RelayAddr:  "45.162.237.152:27610",
		RTT:        42 * time.Millisecond,
		MappedIP:   net.ParseIP("45.162.237.152"),
		MappedPort: 27610,
	}}
	nodes := RelayLatencyTE(results)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 te node, got %d", len(nodes))
	}
	if nodes[0].Attrs["relay_name"] != "fslz10c01" {
		t.Fatalf("unexpected relay_name: %v", nodes[0].Attrs["relay_name"])
	}
	latency := nodes[0].Attrs["latency"].(string)
	if latency != "33554474" {
		t.Fatalf("expected latency 33554474, got %s", latency)
	}
}
