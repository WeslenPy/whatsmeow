// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	waBinary "go.mau.fi/whatsmeow/binary"
	wacallwasm "go.mau.fi/whatsmeow/call/wasm"
	"go.mau.fi/whatsmeow/types"
)

// VoIPStack coordinates WASM engine, relay transport, and signaling bridge.
type VoIPStack struct {
	backend Backend
	bridge  *SignalingBridge
	engine  wacallwasm.Engine
	relay   *RelayTransport
	handler *RelayHandler

	mu          sync.Mutex
	activeCalls map[string]*voipCallContext
}

type voipCallContext struct {
	callID     string
	routeJID   types.JID
	peerDevices []types.JID
}

// NewVoIPStack wires WASM (or relay-only) with Go signaling and UDP relay transport.
func NewVoIPStack(backend Backend, bridge *SignalingBridge, engine wacallwasm.Engine, relay *RelayTransport) *VoIPStack {
	stack := &VoIPStack{
		backend:     backend,
		bridge:      bridge,
		engine:      engine,
		relay:       relay,
		handler:     NewRelayHandler(bridge, 3*defaultAckTimeout/5, backend.LogDebugf),
		activeCalls: make(map[string]*voipCallContext),
	}
	if relay != nil {
		relay.onRecv = stack.handleRelayRecv
	}
	bridge.SetAckHandler(stack.handleSignalingAck)
	return stack
}

// Init initializes the VoIP engine with own JIDs.
func (s *VoIPStack) Init(ctx context.Context) error {
	if s.engine == nil {
		return nil
	}
	return s.engine.Init(ctx, s.backend.GetOwnID(), s.backend.GetOwnLID())
}

// StartOutboundCall starts a WASM-driven outbound call.
func (s *VoIPStack) StartOutboundCall(ctx context.Context, peer *PeerInfo, callID string, isVideo bool) error {
	if s.engine == nil {
		return fmt.Errorf("voip engine not configured")
	}
	tcToken, err := s.backend.EnsureTCToken(ctx, peer.RouteJID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.activeCalls[callID] = &voipCallContext{
		callID:      callID,
		routeJID:    peer.RouteJID,
		peerDevices: peer.PeerDevices,
	}
	s.mu.Unlock()
	return s.engine.StartCall(ctx, wacallwasm.StartCallParams{
		PeerJID:   peer.RouteJID,
		PeerPN:    peer.PeerPN,
		CallID:    callID,
		IsVideo:   isVideo,
		IsLIDCall: true,
		TCToken:   tcToken,
		PeerList:  peer.PeerDevices,
	})
}

// EndCall terminates a VoIP session.
func (s *VoIPStack) EndCall(callID string) error {
	s.mu.Lock()
	delete(s.activeCalls, callID)
	s.mu.Unlock()
	if s.engine == nil {
		return nil
	}
	return s.engine.EndCall(callID, 0, true)
}

// HandleInboundCall forwards inbound signaling to WASM and processes relay nodes.
func (s *VoIPStack) HandleInboundCall(ctx context.Context, node *waBinary.Node, childTag string, meta types.BasicCallMeta, child *waBinary.Node) error {
	if childTag == "offer" || childTag == "accept" {
		if err := s.handler.HandleOfferRelay(ctx, meta.From, meta.CallCreator, meta.CallID, child); err != nil {
			s.backend.LogWarnf("Relay handling for %s failed: %v", childTag, err)
		}
	}
	if s.engine == nil {
		return nil
	}
	payload, err := encodeNodeBase64(child)
	if err != nil {
		return err
	}
	tcToken, _ := s.backend.EnsureTCToken(ctx, meta.From)
	params := wacallwasm.SignalingMessageParams{
		Payload:        payload,
		PeerJID:        meta.From,
		PeerPlatform:   node.AttrGetter().String("platform"),
		PeerAppVersion: node.AttrGetter().String("version"),
		EpochID:        child.AttrGetter().String("e"),
		Timestamp:      child.AttrGetter().String("t"),
		TCToken:        tcToken,
	}
	switch childTag {
	case "offer":
		return s.engine.HandleSignalingOffer(ctx, params)
	default:
		return s.engine.HandleSignalingMessage(ctx, params)
	}
}

// Close shuts down relay transport and WASM engine.
func (s *VoIPStack) Close() error {
	var err error
	if s.relay != nil {
		if closeErr := s.relay.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	if s.engine != nil {
		if closeErr := s.engine.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

func (s *VoIPStack) HandleSignalingReceipt(ctx context.Context, routeJID types.JID, node *waBinary.Node) error {
	if s.engine == nil {
		return nil
	}
	payload, err := encodeNodeBase64(node)
	if err != nil {
		return err
	}
	tcToken, _ := s.backend.EnsureTCToken(ctx, routeJID)
	return s.engine.HandleSignalingReceipt(ctx, wacallwasm.SignalingMessageParams{
		Payload: payload,
		PeerJID: routeJID,
		TCToken: tcToken,
	})
}

func (s *VoIPStack) handleSignalingAck(ctx context.Context, params SignalingAckParams) error {
	if s.engine == nil {
		return nil
	}
	s.backend.LogDebugf("WASM handleSignalingAck msgType=%s ackError=%s peer=%s", params.MsgType, params.AckError, params.PeerJID)
	return s.engine.HandleSignalingAck(ctx, wacallwasm.SignalingAckParams{
		Payload:  params.Payload,
		AckError: params.AckError,
		MsgType:  params.MsgType,
		PeerJID:  params.PeerJID,
		TCToken:  params.TCToken,
	})
}

func (s *VoIPStack) handleRelayRecv(data []byte, ip string, port uint16) {
	if s.engine == nil {
		return
	}
	_ = s.engine.HandleTransportMessage(data, ip, port)
}

// wasmHost implements wasm.HostCallbacks for the VoIP stack.
type wasmHost struct {
	stack *VoIPStack
}

func (h *wasmHost) OnSignalingXmpp(peerJID types.JID, callID string, payload []byte) {
	ctx := context.Background()
	if os.Getenv("VOIP_BRIDGE_DEBUG") == "1" {
		if node, err := decodeSignalingPayload(payload); err == nil {
			tags := make([]string, 0, len(node.GetChildren()))
			for _, ch := range node.GetChildren() {
				tags = append(tags, ch.Tag)
			}
			h.stack.backend.LogDebugf("WASM offer/signaling %s children=%v payloadBytes=%d", node.Tag, tags, len(payload))
		}
	}
	callCtx := h.stack.getCallContext(callID)
	routeJID := peerJID
	var peerDevices []types.JID
	if callCtx != nil {
		routeJID = callCtx.routeJID
		peerDevices = callCtx.peerDevices
	}
	_, err := h.stack.bridge.SendSignalingFromPayload(ctx, routeJID, peerDevices, payload)
	if err != nil {
		h.stack.backend.LogWarnf("WASM signaling send failed for call %s: %v", callID, err)
	}
}

func (h *wasmHost) SendDataToRelay(data []byte, ip string, port uint16) error {
	if h.stack.relay == nil {
		return fmt.Errorf("relay transport not configured")
	}
	return h.stack.relay.SendDataToRelay(data, ip, port)
}

func (h *wasmHost) OnCallEvent(callID string, eventType int, eventData string) {
	if eventType != 156 {
		return
	}
	var relayUpdate struct {
		RelayKey    string              `json:"relay_key"`
		RelayTokens []string            `json:"relay_tokens"`
		AuthTokens  []string            `json:"auth_tokens"`
		Relays      []json.RawMessage   `json:"relays"`
	}
	if err := json.Unmarshal([]byte(eventData), &relayUpdate); err != nil {
		h.stack.backend.LogWarnf("Failed to parse relay list event: %v", err)
		return
	}
	h.stack.backend.LogDebugf("WASM relay list update for call %s (%d relays)", callID, len(relayUpdate.Relays))
}

func (h *wasmHost) LogDebug(msg string, args ...any) {
	h.stack.backend.LogDebugf(msg, args...)
}

func (h *wasmHost) LogWarn(msg string, args ...any) {
	h.stack.backend.LogWarnf(msg, args...)
}

func (s *VoIPStack) getCallContext(callID string) *voipCallContext {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeCalls[callID]
}

// NewVoIPEngine creates the Node sidecar engine or a relay-only fallback.
func NewVoIPEngine(stack *VoIPStack, opts Options) (wacallwasm.Engine, error) {
	host := &wasmHost{stack: stack}
	if opts.EnableWasm {
		return wacallwasm.NewSidecarEngine(wacallwasm.SidecarConfig{
			BridgeScript: opts.SidecarBridgePath,
			Host:         host,
		})
	}
	return wacallwasm.NewRelayEngine(host), nil
}

// EncodeNodeBase64 exports node encoding for external bridges.
func EncodeNodeBase64(node *waBinary.Node) (string, error) {
	data, err := waBinary.Marshal(*node)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
