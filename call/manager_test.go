// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"sync"
	"testing"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type mockBackend struct {
	ownID    types.JID
	ownLID   types.JID
	events   []any
	eventsMu sync.Mutex
}

func (m *mockBackend) GetOwnID() types.JID                          { return m.ownID }
func (m *mockBackend) GetOwnLID() types.JID                         { return m.ownLID }
func (m *mockBackend) GetClientPlatform() string                    { return "web" }
func (m *mockBackend) GetClientVersion() string                     { return "2.3000.0" }
func (m *mockBackend) DispatchEvent(evt any) {
	m.eventsMu.Lock()
	m.events = append(m.events, evt)
	m.eventsMu.Unlock()
}
func (m *mockBackend) LogDebugf(string, ...any)                     {}
func (m *mockBackend) LogWarnf(string, ...any)                      {}
func (m *mockBackend) SendCallNode(context.Context, waBinary.Node) error { return nil }
func (m *mockBackend) ReserveStanzaAck(string) (<-chan *waBinary.Node, func()) {
	ch := make(chan *waBinary.Node, 1)
	ch <- &waBinary.Node{Tag: "ack"}
	return ch, func() {}
}
func (m *mockBackend) WaitReservedStanzaAck(context.Context, <-chan *waBinary.Node, time.Duration) (*waBinary.Node, error) {
	return &waBinary.Node{Tag: "ack"}, nil
}
func (m *mockBackend) ResolveLIDForPN(context.Context, types.JID) (types.JID, error) {
	return types.JID{}, nil
}
func (m *mockBackend) ResolveLIDViaUserInfo(context.Context, types.JID) (types.JID, error) {
	return types.JID{}, nil
}
func (m *mockBackend) GetPeerDevices(context.Context, []types.JID) ([]types.JID, error) {
	return nil, nil
}
func (m *mockBackend) HasSession(context.Context, types.JID) (bool, error) {
	return true, nil
}
func (m *mockBackend) EnsureSignalSessions(context.Context, []types.JID) error {
	return nil
}
func (m *mockBackend) SubscribePresence(context.Context, types.JID) error { return nil }
func (m *mockBackend) EnsureTCToken(context.Context, types.JID) ([]byte, error) {
	return []byte{1, 2, 3}, nil
}
func (m *mockBackend) IssuePrivacyToken(context.Context, types.JID) error { return nil }
func (m *mockBackend) EncryptCallKey(context.Context, types.JID, []byte, int) (*waBinary.Node, bool, error) {
	return &waBinary.Node{Tag: "enc", Attrs: waBinary.Attrs{"v": "2", "type": "msg"}, Content: []byte{9}}, false, nil
}
func (m *mockBackend) EncryptCallKeyForDevices(context.Context, []types.JID, []byte, int) ([]waBinary.Node, bool, error) {
	return nil, false, nil
}
func (m *mockBackend) MakeDeviceIdentityNode() waBinary.Node {
	return waBinary.Node{Tag: "device-identity"}
}
func (m *mockBackend) GenerateStanzaID() string { return "stanza-1" }

func TestManagerHandleInboundPreAccept(t *testing.T) {
	backend := &mockBackend{
		ownID:  types.JID{User: "111", Server: types.DefaultUserServer},
		ownLID: types.JID{User: "111", Server: types.HiddenUserServer},
	}
	manager := NewManager(backend, DefaultOptions())

	session := &OutboundSession{
		CallID:   "00TESTCALL",
		State:    StateOffering,
		PeerPN:   types.JID{User: "222", Server: types.DefaultUserServer},
		RouteJID: types.JID{User: "222", Server: types.HiddenUserServer},
	}
	manager.mu.Lock()
	manager.sessions[session.CallID] = session
	manager.mu.Unlock()

	from, _ := types.ParseJID("222@lid")
	node := &waBinary.Node{
		Tag:   "call",
		Attrs: waBinary.Attrs{"from": from},
		Content: []waBinary.Node{{
			Tag:   "preaccept",
			Attrs: waBinary.Attrs{"call-id": "00TESTCALL", "call-creator": backend.ownLID},
		}},
	}
	meta := types.BasicCallMeta{CallID: "00TESTCALL", From: from}
	if !manager.HandleInbound(context.Background(), node, "preaccept", meta) {
		t.Fatal("expected inbound to be handled")
	}
	if session.getState() != StateRinging {
		t.Fatalf("expected ringing state, got %s", session.getState())
	}

	backend.eventsMu.Lock()
	defer backend.eventsMu.Unlock()
	if len(backend.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(backend.events))
	}
	if _, ok := backend.events[0].(*events.CallRinging); !ok {
		t.Fatalf("expected CallRinging event, got %T", backend.events[0])
	}
}

func TestExtractReceiptCallID(t *testing.T) {
	node := &waBinary.Node{
		Tag: "receipt",
		Content: []waBinary.Node{{
			Tag:   "relaylatency",
			Attrs: waBinary.Attrs{"call-id": "00ABC"},
		}},
	}
	if got := ExtractReceiptCallID(node); got != "00ABC" {
		t.Fatalf("expected 00ABC, got %q", got)
	}
}
