// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

const defaultAckTimeout = 15 * time.Second

// SignalingAckParams is passed to the VoIP engine after a call stanza is acked.
type SignalingAckParams struct {
	Payload  string // base64-encoded ack node
	AckError string
	MsgType  string
	PeerJID  types.JID
	TCToken  []byte
}

// SignalingAckHandler receives server acks for VoIP WASM (relay list allocation).
type SignalingAckHandler func(ctx context.Context, params SignalingAckParams) error

// SignalingBridge encrypts call keys and sends call stanzas to the server.
type SignalingBridge struct {
	backend    Backend
	ackHandler SignalingAckHandler
}

func NewSignalingBridge(backend Backend) *SignalingBridge {
	return &SignalingBridge{backend: backend}
}

// SetAckHandler registers a callback for server acks (required for VoIP relay allocation).
func (b *SignalingBridge) SetAckHandler(handler SignalingAckHandler) {
	b.ackHandler = handler
}

// SendSignalingFromPayload decrypts/encrypts a WASM-produced binary XML payload and sends it.
func (b *SignalingBridge) SendSignalingFromPayload(ctx context.Context, routeJID types.JID, peerDevices []types.JID, payload []byte) (string, error) {
	node, err := decodeSignalingPayload(payload)
	if err != nil {
		return "", err
	}
	prepared, err := b.prepareEncryptedNode(ctx, node, routeJID, peerDevices)
	if err != nil {
		return "", err
	}
	sendTo := routeJID.ToNonAD()
	if prepared.Tag != "offer" && prepared.Tag != "enc_rekey" {
		sendTo = routeJID
	}
	return b.sendCallStanza(ctx, sendTo, prepared, callEnvelopeMeta{
		Platform: b.backend.GetClientPlatform(),
		Version:  b.backend.GetClientVersion(),
	}, prepared.Tag)
}

// SendRelayLatency sends relaylatency with te children after STUN probing.
func (b *SignalingBridge) SendRelayLatency(ctx context.Context, routeJID, callCreator types.JID, callID string, teNodes []waBinary.Node) (string, error) {
	node := &waBinary.Node{
		Tag: "relaylatency",
		Attrs: waBinary.Attrs{
			"call-id":      callID,
			"call-creator": callCreator,
		},
		Content: teNodes,
	}
	return b.sendCallStanza(ctx, routeJID.ToNonAD(), node, callEnvelopeMeta{
		Platform: b.backend.GetClientPlatform(),
		Version:  b.backend.GetClientVersion(),
	}, "relaylatency")
}

// SendOffer encrypts the offer node and sends it to the peer.
func (b *SignalingBridge) SendOffer(ctx context.Context, routeJID types.JID, peerDevices []types.JID, offerNode *waBinary.Node) (stanzaID string, err error) {
	prepared, err := b.prepareEncryptedNode(ctx, offerNode, routeJID, peerDevices)
	if err != nil {
		return "", err
	}
	// Real offers use bare LID routing for the call envelope; enc targets routeJID.
	sendTo := routeJID.ToNonAD()
	return b.sendCallStanza(ctx, sendTo, prepared, callEnvelopeMeta{
		Platform: b.backend.GetClientPlatform(),
		Version:  b.backend.GetClientVersion(),
	}, "offer")
}

// SendTerminate sends a call/terminate stanza.
func (b *SignalingBridge) SendTerminate(ctx context.Context, routeJID, callCreator types.JID, callID, reason string) (string, error) {
	terminateNode := waBinary.Node{
		Tag: "terminate",
		Attrs: waBinary.Attrs{
			"call-id":      callID,
			"call-creator": callCreator,
			"reason":       reason,
			"count":        "0",
		},
	}
	return b.sendCallStanza(ctx, routeJID.ToNonAD(), &terminateNode, callEnvelopeMeta{}, "terminate")
}

type callEnvelopeMeta struct {
	Platform string
	Version  string
}

func (b *SignalingBridge) prepareEncryptedNode(ctx context.Context, voipNode *waBinary.Node, routeJID types.JID, peerDevices []types.JID) (*waBinary.Node, error) {
	node := cloneNode(voipNode)
	if node.Tag != "offer" && node.Tag != "enc_rekey" {
		return &node, nil
	}

	ownLID := b.backend.GetOwnLID()
	if node.Tag == "offer" && node.Attrs["call-creator"] == nil && !ownLID.IsEmpty() {
		node.Attrs["call-creator"] = ownLID
	}

	destNode, hasDest := node.GetOptionalChildByTag("destination")
	if hasDest {
		includeIdentity, err := b.encryptDestinationEncNodes(ctx, &destNode, peerDevices)
		if err != nil {
			return nil, err
		}
		replaceChild(&node, "destination", destNode)
		removeChild(&node, "enc")
		if includeIdentity {
			appendChild(&node, b.backend.MakeDeviceIdentityNode())
		}
		return &node, nil
	}

	enc := node.GetChildByTag("enc")
	rawKey, ok := enc.Content.([]byte)
	if !ok || len(rawKey) == 0 {
		return &node, nil
	}
	encrypted, includeIdentity, err := b.backend.EncryptCallKey(ctx, routeJID, rawKey, parseCountAttr(enc.Attrs["count"]))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt call key: %w", err)
	}
	replaceChild(&node, "enc", *encrypted)
	if includeIdentity {
		appendChild(&node, b.backend.MakeDeviceIdentityNode())
	}
	return &node, nil
}

func (b *SignalingBridge) encryptDestinationEncNodes(ctx context.Context, dest *waBinary.Node, peerDevices []types.JID) (bool, error) {
	includeIdentity := false
	for i, child := range dest.GetChildren() {
		if child.Tag != "to" {
			continue
		}
		targetJID, _ := child.Attrs["jid"].(types.JID)
		if targetJID.IsEmpty() {
			continue
		}
		enc := child.GetChildByTag("enc")
		rawKey, ok := enc.Content.([]byte)
		if !ok || len(rawKey) == 0 {
			continue
		}
		encrypted, isPreKey, err := b.backend.EncryptCallKey(ctx, targetJID, rawKey, parseCountAttr(enc.Attrs["count"]))
		if err != nil {
			return false, fmt.Errorf("failed to encrypt call key for %s: %w", targetJID, err)
		}
		children := child.GetChildren()
		for j := range children {
			if children[j].Tag == "enc" {
				children[j] = *encrypted
				break
			}
		}
		child.Content = children
		destChildren := dest.GetChildren()
		destChildren[i] = child
		dest.Content = destChildren
		if isPreKey {
			includeIdentity = true
		}
	}
	_ = peerDevices
	return includeIdentity, nil
}

func (b *SignalingBridge) sendCallStanza(ctx context.Context, routeJID types.JID, voipNode *waBinary.Node, meta callEnvelopeMeta, signalingTag string) (string, error) {
	stanzaID := b.backend.GenerateStanzaID()
	waiter, release := b.backend.ReserveStanzaAck(stanzaID)
	ownID := b.backend.GetOwnID()
	attrs := waBinary.Attrs{"id": stanzaID, "from": ownID, "to": routeJID}
	if meta.Platform != "" {
		attrs["platform"] = meta.Platform
	}
	if meta.Version != "" {
		attrs["version"] = meta.Version
	}
	err := b.backend.SendCallNode(ctx, waBinary.Node{
		Tag:     "call",
		Attrs:   attrs,
		Content: []waBinary.Node{*voipNode},
	})
	if err != nil {
		release()
		return "", err
	}

	deliverAck := func(ack *waBinary.Node, ackErr error) {
		defer release()
		if ackErr != nil {
			b.backend.LogWarnf("Call stanza %s ack wait failed: %v", stanzaID, ackErr)
			return
		}
		if ack == nil {
			return
		}
		ackErrCode := "0"
		if ack.Attrs != nil {
			if v, ok := ack.Attrs["error"]; ok {
				ackErrCode = fmt.Sprint(v)
			}
		}
		if ackErrCode != "0" && ackErrCode != "" {
			b.backend.LogWarnf("Call stanza %s ack error: %s", stanzaID, ackErrCode)
		} else {
			b.backend.LogDebugf("Call stanza %s ack received", stanzaID)
		}
		if b.ackHandler == nil {
			return
		}
		payload, encErr := encodeNodeBase64(ack)
		if encErr != nil {
			b.backend.LogWarnf("Failed to encode ack for VoIP: %v", encErr)
			return
		}
		msgType := signalingTag
		if ack.Attrs != nil {
			if v, ok := ack.Attrs["type"]; ok {
				msgType = fmt.Sprint(v)
			}
		}
		tcToken, _ := b.backend.EnsureTCToken(ctx, routeJID)
		_ = b.ackHandler(ctx, SignalingAckParams{
			Payload:  payload,
			AckError: ackErrCode,
			MsgType:  msgType,
			PeerJID:  routeJID,
			TCToken:  tcToken,
		})
	}

	// Ack delivery is async (matches baileys-caller) so outbound signaling is not blocked.
	go func() {
		ackCtx, cancel := context.WithTimeout(context.Background(), defaultAckTimeout)
		defer cancel()
		ack, ackErr := b.backend.WaitReservedStanzaAck(ackCtx, waiter, defaultAckTimeout)
		deliverAck(ack, ackErr)
	}()

	return stanzaID, nil
}

func decodeSignalingPayload(payload []byte) (*waBinary.Node, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty signaling payload")
	}
	// baileys-caller prepends a 0 byte before binary XML decode.
	withPrefix := append([]byte{0}, payload...)
	node, err := waBinary.Unmarshal(withPrefix)
	if err != nil {
		data := payload
		if data[0] == 0 {
			data = data[1:]
		}
		node, err = waBinary.Unmarshal(data)
	}
	if err != nil {
		node, err = waBinary.Unmarshal(payload)
	}
	if err != nil {
		return nil, fmt.Errorf("decode signaling payload: %w", err)
	}
	return node, nil
}

func encodeNodeBase64(node *waBinary.Node) (string, error) {
	data, err := waBinary.Marshal(*node)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func parseCountAttr(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case string:
		var parsed int
		fmt.Sscanf(v, "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func replaceChild(node *waBinary.Node, tag string, child waBinary.Node) {
	children := node.GetChildren()
	for i := range children {
		if children[i].Tag == tag {
			children[i] = child
			node.Content = children
			return
		}
	}
	children = append(children, child)
	node.Content = children
}

func appendChild(node *waBinary.Node, child waBinary.Node) {
	children := node.GetChildren()
	node.Content = append(children, child)
}

func removeChild(node *waBinary.Node, tag string) {
	children := node.GetChildren()
	filtered := make([]waBinary.Node, 0, len(children))
	for i := range children {
		if children[i].Tag != tag {
			filtered = append(filtered, children[i])
		}
	}
	node.Content = filtered
}
