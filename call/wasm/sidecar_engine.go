// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package wasm

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/call/sidecar"
	"go.mau.fi/whatsmeow/types"
)

const sidecarInitTimeout = 120 * time.Second

// SidecarConfig configures the Node.js WASM bridge subprocess.
type SidecarConfig struct {
	NodeCommand  string
	BridgeScript string
	RepoRoot     string
	Host         HostCallbacks
}

// SidecarEngine implements Engine via the Node voip-bridge subprocess.
type SidecarEngine struct {
	cfg    SidecarConfig
	client *sidecar.Client
}

// NewSidecarEngine creates an engine backed by cmd/voip-bridge/bridge.mjs.
func NewSidecarEngine(cfg SidecarConfig) (*SidecarEngine, error) {
	if cfg.Host == nil {
		return nil, fmt.Errorf("sidecar host callbacks are required")
	}
	return &SidecarEngine{cfg: cfg}, nil
}

// Init starts the Node subprocess and initializes the VoIP stack.
func (e *SidecarEngine) Init(ctx context.Context, ownPN, ownLID types.JID) error {
	host := e.cfg.Host
	bridgeScript := e.cfg.BridgeScript
	if bridgeScript == "" {
		bridgeScript = sidecar.DefaultBridgeScript()
	}
	client, err := sidecar.Start(ctx, sidecar.Config{
		NodeCommand:  e.cfg.NodeCommand,
		BridgeScript: bridgeScript,
		RepoRoot:     e.cfg.RepoRoot,
		OnSignalingXmpp: func(peerJID types.JID, callID string, payload []byte) {
			host.OnSignalingXmpp(peerJID, callID, payload)
		},
		OnCallEvent: func(callID string, eventType int, eventData string) {
			host.OnCallEvent(callID, eventType, eventData)
		},
		SendDataToRelay: func(data []byte, ip string, port uint16) error {
			return host.SendDataToRelay(data, ip, port)
		},
		LogDebug: host.LogDebug,
		LogWarn:  host.LogWarn,
	})
	if err != nil {
		return fmt.Errorf("start voip sidecar: %w", err)
	}
	e.client = client
	initCtx, cancel := context.WithTimeout(ctx, sidecarInitTimeout)
	defer cancel()
	return client.Init(initCtx, ownPN, ownLID)
}

func (e *SidecarEngine) StartCall(ctx context.Context, params StartCallParams) error {
	if e.client == nil {
		return fmt.Errorf("sidecar not initialized")
	}
	return e.client.StartCall(ctx, sidecar.StartCallParams{
		PeerJID:   params.PeerJID,
		PeerPN:    params.PeerPN,
		CallID:    params.CallID,
		IsVideo:   params.IsVideo,
		IsLIDCall: params.IsLIDCall,
		TCToken:   params.TCToken,
		PeerList:  params.PeerList,
	})
}

func (e *SidecarEngine) EndCall(callID string, reason int, sendTerminate bool) error {
	if e.client == nil {
		return nil
	}
	return e.client.EndCall(callID, reason, sendTerminate)
}

func (e *SidecarEngine) HandleSignalingAck(ctx context.Context, params SignalingAckParams) error {
	if e.client == nil {
		return nil
	}
	return e.client.HandleSignalingAck(ctx, sidecar.SignalingAckParams{
		Payload:  params.Payload,
		AckError: params.AckError,
		MsgType:  params.MsgType,
		PeerJID:  params.PeerJID,
		TCToken:  params.TCToken,
	})
}

func (e *SidecarEngine) HandleSignalingMessage(ctx context.Context, params SignalingMessageParams) error {
	if e.client == nil {
		return nil
	}
	return e.client.HandleSignalingMessage(ctx, sidecar.SignalingMessageParams{
		Payload:        params.Payload,
		PeerJID:        params.PeerJID,
		PeerPlatform:   params.PeerPlatform,
		PeerAppVersion: params.PeerAppVersion,
		EpochID:        params.EpochID,
		Timestamp:      params.Timestamp,
		IsOffline:      params.IsOffline,
		TCToken:        params.TCToken,
	})
}

func (e *SidecarEngine) HandleSignalingOffer(ctx context.Context, params SignalingMessageParams) error {
	if e.client == nil {
		return nil
	}
	return e.client.HandleSignalingOffer(ctx, sidecar.SignalingMessageParams{
		Payload:        params.Payload,
		PeerJID:        params.PeerJID,
		PeerPlatform:   params.PeerPlatform,
		PeerAppVersion: params.PeerAppVersion,
		EpochID:        params.EpochID,
		Timestamp:      params.Timestamp,
		IsOffline:      params.IsOffline,
		TCToken:        params.TCToken,
	})
}

func (e *SidecarEngine) HandleSignalingReceipt(ctx context.Context, params SignalingMessageParams) error {
	if e.client == nil {
		return nil
	}
	return e.client.HandleSignalingReceipt(ctx, sidecar.SignalingMessageParams{
		Payload: params.Payload,
		PeerJID: params.PeerJID,
		TCToken: params.TCToken,
	})
}

func (e *SidecarEngine) HandleTransportMessage(data []byte, ip string, port uint16) error {
	if e.client == nil {
		return nil
	}
	return e.client.HandleTransportMessage(data, ip, port)
}

func (e *SidecarEngine) Close() error {
	if e.client == nil {
		return nil
	}
	return e.client.Close()
}
