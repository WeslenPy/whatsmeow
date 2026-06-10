// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package wasm

import (
	"context"
	"fmt"
	"sync"

	"go.mau.fi/whatsmeow/types"
)

// RelayEngine implements Engine using Go-native relay transport only (no WASM).
// Useful for inbound relay handling and as a fallback when WASM is unavailable.
type RelayEngine struct {
	host HostCallbacks
	mu   sync.Mutex
}

// NewRelayEngine creates a relay-only VoIP engine without WASM.
func NewRelayEngine(host HostCallbacks) *RelayEngine {
	return &RelayEngine{host: host}
}

func (e *RelayEngine) Init(ctx context.Context, ownPN, ownLID types.JID) error {
	_ = ctx
	_ = ownPN
	_ = ownLID
	return nil
}

func (e *RelayEngine) StartCall(ctx context.Context, params StartCallParams) error {
	return fmt.Errorf("relay-only engine cannot start outbound calls without WASM")
}

func (e *RelayEngine) EndCall(callID string, reason int, sendTerminate bool) error {
	_ = callID
	_ = reason
	_ = sendTerminate
	return nil
}

func (e *RelayEngine) HandleSignalingAck(ctx context.Context, params SignalingAckParams) error {
	_ = ctx
	_ = params
	return nil
}

func (e *RelayEngine) HandleSignalingMessage(ctx context.Context, params SignalingMessageParams) error {
	_ = ctx
	_ = params
	return nil
}

func (e *RelayEngine) HandleSignalingOffer(ctx context.Context, params SignalingMessageParams) error {
	_ = ctx
	_ = params
	return nil
}

func (e *RelayEngine) HandleSignalingReceipt(ctx context.Context, params SignalingMessageParams) error {
	_ = ctx
	_ = params
	return nil
}

func (e *RelayEngine) HandleTransportMessage(data []byte, ip string, port uint16) error {
	if e.host != nil {
		return e.host.SendDataToRelay(data, ip, port)
	}
	return nil
}

func (e *RelayEngine) Close() error {
	return nil
}
