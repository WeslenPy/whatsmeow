// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/session"
	"google.golang.org/protobuf/proto"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/call"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type callBackend struct {
	cli *Client
}

func (b *callBackend) GetOwnID() types.JID {
	return b.cli.getOwnID()
}

func (b *callBackend) GetOwnLID() types.JID {
	return b.cli.getOwnLID()
}

func (b *callBackend) GetClientPlatform() string {
	return "web"
}

func (b *callBackend) GetClientVersion() string {
	return store.GetWAVersion().String()
}

func (b *callBackend) DispatchEvent(evt any) {
	b.cli.dispatchEvent(evt)
}

func (b *callBackend) LogDebugf(msg string, args ...any) {
	b.cli.Log.Debugf(msg, args...)
}

func (b *callBackend) LogWarnf(msg string, args ...any) {
	b.cli.Log.Warnf(msg, args...)
}

func (b *callBackend) SendCallNode(ctx context.Context, node waBinary.Node) error {
	return b.cli.sendNode(ctx, node)
}

func (b *callBackend) ReserveStanzaAck(stanzaID string) (<-chan *waBinary.Node, func()) {
	waiter := b.cli.waitResponse(stanzaID)
	return waiter, func() {
		b.cli.cancelResponse(stanzaID, waiter)
	}
}

func (b *callBackend) WaitReservedStanzaAck(ctx context.Context, waiter <-chan *waBinary.Node, timeout time.Duration) (*waBinary.Node, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	select {
	case res := <-waiter:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, ErrIQTimedOut
	}
}

func (b *callBackend) ResolveLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	if b.cli.Store == nil || b.cli.Store.LIDs == nil {
		return types.JID{}, nil
	}
	return b.cli.Store.LIDs.GetLIDForPN(ctx, pn)
}

func (b *callBackend) ResolveLIDViaUserInfo(ctx context.Context, pn types.JID) (types.JID, error) {
	info, err := b.cli.GetUserInfo(ctx, []types.JID{pn.ToNonAD()})
	if err != nil {
		return types.JID{}, err
	}
	userInfo, ok := info[pn.ToNonAD()]
	if !ok {
		return types.JID{}, nil
	}
	return userInfo.LID, nil
}

func (b *callBackend) GetPeerDevices(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	return b.cli.GetUserDevices(ctx, jids)
}

func (b *callBackend) HasSession(ctx context.Context, jid types.JID) (bool, error) {
	return b.cli.Store.ContainsSession(ctx, jid.SignalAddress())
}

func (b *callBackend) EnsureSignalSessions(ctx context.Context, devices []types.JID) error {
	var missing []types.JID
	for _, jid := range devices {
		exists, err := b.cli.Store.ContainsSession(ctx, jid.SignalAddress())
		if err != nil {
			return fmt.Errorf("failed to check session for %s: %w", jid, err)
		}
		if !exists {
			missing = append(missing, jid)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	bundles := b.cli.fetchPreKeysNoError(ctx, missing)
	for _, jid := range missing {
		bundle := bundles[jid]
		if bundle == nil {
			return fmt.Errorf("no prekey bundle available for %s", jid)
		}
		builder := session.NewBuilderFromSignal(b.cli.Store, jid.SignalAddress(), pbSerializer)
		if err := builder.ProcessBundle(ctx, bundle); err != nil {
			return fmt.Errorf("failed to establish session with %s: %w", jid, err)
		}
	}
	return nil
}

func (b *callBackend) SubscribePresence(ctx context.Context, jid types.JID) error {
	return b.cli.SubscribePresence(ctx, jid)
}

func (b *callBackend) EnsureTCToken(ctx context.Context, jid types.JID) ([]byte, error) {
	return b.cli.EnsureTCToken(ctx, jid)
}

func (b *callBackend) IssuePrivacyToken(ctx context.Context, jid types.JID) error {
	_, err := b.cli.IssuePrivacyToken(ctx, jid, time.Now())
	return err
}

func (b *callBackend) EncryptCallKey(ctx context.Context, target types.JID, callKey []byte, count int) (*waBinary.Node, bool, error) {
	msg := &waE2E.Message{Call: &waE2E.Call{CallKey: callKey}}
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal call message: %w", err)
	}
	encAttrs := waBinary.Attrs{}
	if count > 0 {
		encAttrs["count"] = count
	}
	var bundle *prekey.Bundle
	exists, err := b.cli.Store.ContainsSession(ctx, target.SignalAddress())
	if err != nil {
		return nil, false, fmt.Errorf("failed to check session for %s: %w", target, err)
	} else if !exists {
		bundles := b.cli.fetchPreKeysNoError(ctx, []types.JID{target})
		bundle = bundles[target]
		if bundle == nil {
			return nil, false, fmt.Errorf("no prekey bundle available for %s", target)
		}
	}
	node, includeIdentity, err := b.cli.encryptMessageForDevice(ctx, plaintext, target, bundle, encAttrs, nil)
	if err != nil {
		return nil, false, err
	}
	return node, includeIdentity, nil
}

func (b *callBackend) EncryptCallKeyForDevices(ctx context.Context, devices []types.JID, callKey []byte, count int) ([]waBinary.Node, bool, error) {
	msg := &waE2E.Message{Call: &waE2E.Call{CallKey: callKey}}
	plaintext, err := proto.Marshal(msg)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal call message: %w", err)
	}
	encAttrs := waBinary.Attrs{}
	if count > 0 {
		encAttrs["count"] = count
	}
	id := b.cli.GenerateMessageID()
	nodes, includeIdentity, err := b.cli.encryptMessageForDevices(ctx, devices, id, plaintext, nil, encAttrs)
	return nodes, includeIdentity, err
}

func (b *callBackend) MakeDeviceIdentityNode() waBinary.Node {
	return b.cli.makeDeviceIdentityNode()
}

func (b *callBackend) GenerateStanzaID() string {
	return b.cli.GenerateMessageID()
}

// EnableCallSupport initializes outbound call signaling support on the client.
func (cli *Client) EnableCallSupport(opts call.Options) {
	if cli.callManager == nil {
		cli.callManager = call.NewManager(&callBackend{cli: cli}, opts)
	}
}

// EnableVoIP enables relay transport and Node.js WASM sidecar VoIP.
// Requires Node >= 20 and `npm run setup` in cmd/voip-bridge.
func (cli *Client) EnableVoIP(ctx context.Context) error {
	cli.EnableCallSupport(call.Options{
		RingingTimeout: call.DefaultOptions().RingingTimeout,
		EnableRelay:    true,
		EnableWasm:     true,
	})
	return cli.callManager.InitVoIP(ctx)
}

func (cli *Client) ensureCallManager() error {
	if cli.callManager == nil {
		cli.EnableCallSupport(call.DefaultOptions())
	}
	return nil
}

// OutgoingCallOptions configures InitiateCall.
type OutgoingCallOptions struct {
	IsVideo bool
}

// InitiateCall starts an outbound call (signaling only; no audio in this phase).
func (cli *Client) InitiateCall(ctx context.Context, to types.JID, opts OutgoingCallOptions) (*call.OutboundCall, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	} else if !cli.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}
	if err := cli.ensureCallManager(); err != nil {
		return nil, err
	}
	return cli.callManager.InitiateCall(ctx, to, opts.IsVideo)
}

// DumpCallNode serializes a call binary node to JSON for protocol analysis.
func DumpCallNode(node *waBinary.Node) ([]byte, error) {
	return call.DumpNode(node)
}
