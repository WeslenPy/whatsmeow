// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"fmt"
	"sync"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Options configures outbound call behavior.
type Options struct {
	RingingTimeout time.Duration
	// EnableWasm enables the Node.js VoIP sidecar (cmd/voip-bridge).
	EnableWasm bool
	// SidecarBridgePath overrides cmd/voip-bridge/bridge.mjs.
	SidecarBridgePath string
	// EnableRelay turns on UDP relay transport and STUN relaylatency handling.
	EnableRelay bool
}

// DefaultOptions returns sensible defaults for outbound calls.
func DefaultOptions() Options {
	return Options{RingingTimeout: 60 * time.Second}
}

// Manager coordinates outbound call sessions and signaling.
type Manager struct {
	backend Backend
	opts    Options

	preparer *PeerPreparer
	bridge   *SignalingBridge
	builder  *OfferBuilder
	voip     *VoIPStack

	mu       sync.RWMutex
	sessions map[string]*OutboundSession
	sendMu   sync.Mutex
}

// NewManager creates a call manager for outbound signaling.
func NewManager(backend Backend, opts Options) *Manager {
	if opts.RingingTimeout <= 0 {
		opts.RingingTimeout = DefaultOptions().RingingTimeout
	}
	m := &Manager{
		backend:  backend,
		opts:     opts,
		preparer: NewPeerPreparer(backend),
		bridge:   NewSignalingBridge(backend),
		builder:  NewOfferBuilder(),
		sessions: make(map[string]*OutboundSession),
	}
	if opts.EnableRelay || opts.EnableWasm {
		relay := NewRelayTransport(RelayTransportConfig{Logf: backend.LogDebugf})
		stack := NewVoIPStack(backend, m.bridge, nil, relay)
		engine, err := NewVoIPEngine(stack, opts)
		if err != nil {
			backend.LogWarnf("VoIP WASM engine unavailable: %v (relay-only mode)", err)
		} else {
			stack.engine = engine
		}
		m.voip = stack
	}
	return m
}

// InitVoIP initializes the Node WASM sidecar and relay stack.
func (m *Manager) InitVoIP(ctx context.Context) error {
	if m.voip == nil {
		return nil
	}
	return m.voip.Init(ctx)
}

// InitiateCall starts an outbound voice call (signaling only).
func (m *Manager) InitiateCall(ctx context.Context, to types.JID, isVideo bool) (*OutboundCall, error) {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()

	peer, err := m.preparer.Prepare(ctx, to)
	if err != nil {
		return nil, err
	}

	callID, err := GenerateCallID()
	if err != nil {
		return nil, err
	}

	ownLID := m.backend.GetOwnLID()
	if ownLID.IsEmpty() {
		ownLID = m.backend.GetOwnID()
	}

	var offerNode *waBinary.Node
	if m.voip != nil && m.voip.engine != nil && m.opts.EnableWasm {
		if err := m.voip.StartOutboundCall(ctx, peer, callID, isVideo); err != nil {
			return nil, fmt.Errorf("wasm start call: %w", err)
		}
		offerNode = &waBinary.Node{Tag: "offer", Attrs: waBinary.Attrs{"call-id": callID}}
	} else {
		var buildErr error
		offerNode, _, buildErr = m.builder.Build(OfferParams{
			CallID:      callID,
			CallCreator: ownLID.ToNonAD(),
			CallerPN:    m.backend.GetOwnID().ToNonAD(),
			RouteJID:    peer.RouteJID,
			IsVideo:     isVideo,
			Joinable:    true,
		})
		if buildErr != nil {
			return nil, buildErr
		}
	}

	session := &OutboundSession{
		CallID:    callID,
		PeerPN:    peer.PeerPN,
		PeerLID:   peer.PeerLID,
		RouteJID:  peer.RouteJID,
		IsVideo:   isVideo,
		State:     StateOffering,
		CreatedAt: time.Now(),
	}
	session.capture("outbound", "offer", offerNode)

	m.mu.Lock()
	m.sessions[callID] = session
	m.mu.Unlock()

	if m.voip == nil || !m.opts.EnableWasm {
		_, err = m.bridge.SendOffer(ctx, peer.RouteJID, peer.PeerDevices, offerNode)
		if err != nil {
			m.removeSession(callID)
			return nil, fmt.Errorf("failed to send call offer: %w", err)
		}
	}

	session.setState(StateOffering)
	m.backend.DispatchEvent(&events.CallOutgoing{
		BasicCallMeta: types.BasicCallMeta{
			From:        m.backend.GetOwnID(),
			Timestamp:   time.Now(),
			CallCreator: ownLID.ToNonAD(),
			CallID:      callID,
		},
		PeerJID: peer.PeerPN,
		Data:    offerNode,
	})

	outbound := &OutboundCall{
		manager: m,
		session: session,
		endCh:   make(chan string, 1),
	}
	session.onEnd = outbound.notifyEnded
	outbound.startRingingTimeout(ctx)
	return outbound, nil
}

// HandleInbound processes inbound call stanzas for active outbound sessions.
func (m *Manager) HandleInbound(ctx context.Context, node *waBinary.Node, childTag string, meta types.BasicCallMeta) bool {
	session := m.getSession(meta.CallID)
	if session == nil {
		return false
	}

	child := node.GetChildren()[0]
	session.capture("inbound", childTag, &child)

	switch childTag {
	case "preaccept":
		if session.getState() == StateOffering {
			session.setState(StateRinging)
			m.backend.DispatchEvent(&events.CallRinging{
				BasicCallMeta: meta,
				Data:          &child,
			})
		}
	case "accept":
		session.setState(StateSignalingConnected)
		session.ConnectedAt = time.Now()
		if m.voip != nil {
			_ = m.voip.HandleInboundCall(ctx, node, childTag, meta, &child)
		}
		m.backend.DispatchEvent(&events.CallSignalingConnected{
			BasicCallMeta: meta,
			Data:          &child,
		})
	case "transport", "relaylatency":
		m.backend.LogDebugf("Captured inbound %s for call %s (media phase)", childTag, meta.CallID)
		if m.voip != nil {
			_ = m.voip.HandleInboundCall(ctx, node, childTag, meta, &child)
		}
	case "terminate":
		reason := child.AttrGetter().String("reason")
		m.endSession(session, reason)
	case "reject":
		m.endSession(session, "rejected")
	}
	return true
}

// HandleReceipt processes call-related receipts for active outbound sessions.
func (m *Manager) HandleReceipt(ctx context.Context, node *waBinary.Node, callID string) bool {
	session := m.getSession(callID)
	if session == nil {
		return false
	}
	session.capture("inbound", "receipt", node)
	m.backend.LogDebugf("Call receipt for %s", callID)
	if m.voip != nil {
		if err := m.voip.HandleSignalingReceipt(ctx, session.RouteJID, node); err != nil {
			m.backend.LogWarnf("WASM receipt handling for %s failed: %v", callID, err)
		}
	}
	return true
}

// HasSession reports whether an outbound session exists for the call ID.
func (m *Manager) HasSession(callID string) bool {
	return m.getSession(callID) != nil
}

func (m *Manager) cancelSession(ctx context.Context, session *OutboundSession, reason string) error {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()

	if session.getState() == StateEnded {
		return nil
	}
	ownLID := m.backend.GetOwnLID()
	if ownLID.IsEmpty() {
		ownLID = m.backend.GetOwnID()
	}
	_, err := m.bridge.SendTerminate(ctx, session.RouteJID, ownLID.ToNonAD(), session.CallID, reason)
	m.endSession(session, reason)
	return err
}

func (m *Manager) endSession(session *OutboundSession, reason string) {
	if session.getState() == StateEnded {
		return
	}
	session.setState(StateEnded)
	session.EndReason = reason
	if session.onEnd != nil {
		session.onEnd(reason)
	}
	m.removeSession(session.CallID)
	m.backend.DispatchEvent(&events.CallEnded{
		BasicCallMeta: types.BasicCallMeta{
			From:        session.PeerPN,
			Timestamp:   time.Now(),
			CallCreator: session.PeerLID,
			CallID:      session.CallID,
		},
		Reason: reason,
	})
}

func (m *Manager) getSession(callID string) *OutboundSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[callID]
}

func (m *Manager) removeSession(callID string) {
	m.mu.Lock()
	delete(m.sessions, callID)
	m.mu.Unlock()
}
