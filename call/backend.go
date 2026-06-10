// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// Backend exposes whatsmeow client capabilities required for call signaling.
type Backend interface {
	GetOwnID() types.JID
	GetOwnLID() types.JID
	GetClientPlatform() string
	GetClientVersion() string

	DispatchEvent(evt any)
	LogDebugf(msg string, args ...any)
	LogWarnf(msg string, args ...any)

	SendCallNode(ctx context.Context, node waBinary.Node) error
	ReserveStanzaAck(stanzaID string) (waiter <-chan *waBinary.Node, release func())
	WaitReservedStanzaAck(ctx context.Context, waiter <-chan *waBinary.Node, timeout time.Duration) (*waBinary.Node, error)

	ResolveLIDForPN(ctx context.Context, pn types.JID) (types.JID, error)
	ResolveLIDViaUserInfo(ctx context.Context, pn types.JID) (types.JID, error)
	GetPeerDevices(ctx context.Context, jids []types.JID) ([]types.JID, error)
	HasSession(ctx context.Context, jid types.JID) (bool, error)
	EnsureSignalSessions(ctx context.Context, devices []types.JID) error
	SubscribePresence(ctx context.Context, jid types.JID) error
	EnsureTCToken(ctx context.Context, jid types.JID) ([]byte, error)
	IssuePrivacyToken(ctx context.Context, jid types.JID) error

	EncryptCallKey(ctx context.Context, target types.JID, callKey []byte, count int) (*waBinary.Node, bool, error)
	EncryptCallKeyForDevices(ctx context.Context, devices []types.JID, callKey []byte, count int) (destinationNodes []waBinary.Node, includeIdentity bool, err error)
	MakeDeviceIdentityNode() waBinary.Node

	GenerateStanzaID() string
}
