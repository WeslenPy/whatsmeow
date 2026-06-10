// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"fmt"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// RelayHandler coordinates STUN probing and relaylatency signaling.
type RelayHandler struct {
	bridge  *SignalingBridge
	timeout time.Duration
	logf    func(msg string, args ...any)
}

// NewRelayHandler creates a relay handler that pings edge servers and reports latency.
func NewRelayHandler(bridge *SignalingBridge, timeout time.Duration, logf func(msg string, args ...any)) *RelayHandler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &RelayHandler{bridge: bridge, timeout: timeout, logf: logf}
}

// HandleOfferRelay parses relay from an offer/accept node, pings relays, and sends relaylatency.
func (h *RelayHandler) HandleOfferRelay(ctx context.Context, routeJID, callCreator types.JID, callID string, voipChild *waBinary.Node) error {
	relayNode := voipChild.GetChildByTag("relay")
	if relayNode.Tag == "" {
		return nil
	}
	parsed, err := ParseRelayNode(&relayNode)
	if err != nil {
		return fmt.Errorf("parse relay: %w", err)
	}
	return h.PingAndReport(ctx, routeJID, callCreator, callID, parsed)
}

// PingAndReport STUN-pings relay endpoints and sends relaylatency stanzas.
func (h *RelayHandler) PingAndReport(ctx context.Context, routeJID, callCreator types.JID, callID string, relay *ParsedRelay) error {
	if relay == nil {
		return nil
	}
	results := PingRelays(relay, h.timeout)
	for _, r := range results {
		h.logf("Relay %s RTT %v mapped %s:%d", r.RelayName, r.RTT, r.MappedIP, r.MappedPort)
	}
	teNodes := RelayLatencyTE(results)
	if len(teNodes) == 0 {
		return nil
	}
	_, err := h.bridge.SendRelayLatency(ctx, routeJID, callCreator, callID, teNodes)
	return err
}
