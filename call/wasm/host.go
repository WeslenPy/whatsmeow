// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package wasm defines the VoIP engine interface and Node sidecar adapter.
package wasm

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

// HostCallbacks are invoked by the WASM module (mirrors baileys-caller callbacks).
type HostCallbacks interface {
	OnSignalingXmpp(peerJID types.JID, callID string, payload []byte)
	SendDataToRelay(data []byte, ip string, port uint16) error
	OnCallEvent(callID string, eventType int, eventData string)
	LogDebug(msg string, args ...any)
	LogWarn(msg string, args ...any)
}

// StartCallParams configures an outbound VoIP call.
type StartCallParams struct {
	PeerJID   types.JID
	PeerPN    types.JID
	CallID    string
	IsVideo   bool
	IsLIDCall bool
	TCToken   []byte
	PeerList  []types.JID
}

// SignalingAckParams mirrors call.SignalingAckParams for the WASM boundary.
type SignalingAckParams struct {
	Payload  string
	AckError string
	MsgType  string
	PeerJID  types.JID
	TCToken  []byte
}

// SignalingMessageParams is an inbound or outbound signaling stanza for WASM.
type SignalingMessageParams struct {
	Payload        string
	PeerJID        types.JID
	PeerPlatform   string
	PeerAppVersion string
	EpochID        string
	Timestamp      string
	IsOffline      bool
	TCToken        []byte
}

// Engine is the VoIP WASM runtime interface.
type Engine interface {
	Init(ctx context.Context, ownPN, ownLID types.JID) error
	StartCall(ctx context.Context, params StartCallParams) error
	EndCall(callID string, reason int, sendTerminate bool) error
	HandleSignalingAck(ctx context.Context, params SignalingAckParams) error
	HandleSignalingMessage(ctx context.Context, params SignalingMessageParams) error
	HandleSignalingOffer(ctx context.Context, params SignalingMessageParams) error
	HandleSignalingReceipt(ctx context.Context, params SignalingMessageParams) error
	HandleTransportMessage(data []byte, ip string, port uint16) error
	Close() error
}
