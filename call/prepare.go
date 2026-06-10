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

	"go.mau.fi/whatsmeow/types"
)

const defaultPresenceWait = 750 * time.Millisecond

// PeerInfo contains resolved peer data required before placing a call.
type PeerInfo struct {
	PeerPN      types.JID
	PeerLID     types.JID
	RouteJID    types.JID
	PeerDevices []types.JID
	TCToken     []byte
}

// PeerPreparer resolves LID, devices, sessions and TC tokens for outbound calls.
type PeerPreparer struct {
	backend Backend
}

func NewPeerPreparer(backend Backend) *PeerPreparer {
	return &PeerPreparer{backend: backend}
}

// Prepare runs the pre-call setup sequence from baileys-caller.
func (p *PeerPreparer) Prepare(ctx context.Context, peerPN types.JID) (*PeerInfo, error) {
	peerPN = peerPN.ToNonAD()
	if peerPN.Server != types.DefaultUserServer {
		return nil, fmt.Errorf("outbound calls require a phone number JID (@s.whatsapp.net), got %s", peerPN)
	}

	peerLID, err := p.backend.ResolveLIDForPN(ctx, peerPN)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LID: %w", err)
	}
	if peerLID.IsEmpty() {
		peerLID, err = p.backend.ResolveLIDViaUserInfo(ctx, peerPN)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve LID via user info: %w", err)
		}
	}
	if peerLID.IsEmpty() {
		return nil, fmt.Errorf("no LID mapping for %s; try fetching user info first", peerPN)
	}
	peerLID = peerLID.ToNonAD()

	if err = p.backend.SubscribePresence(ctx, peerPN); err != nil {
		p.backend.LogWarnf("Failed to subscribe presence for %s: %v", peerPN, err)
	}
	if err = p.backend.SubscribePresence(ctx, peerLID); err != nil {
		p.backend.LogWarnf("Failed to subscribe presence for %s: %v", peerLID, err)
	}
	if err = waitContext(ctx, defaultPresenceWait); err != nil {
		return nil, err
	}

	devices, err := p.backend.GetPeerDevices(ctx, []types.JID{peerLID})
	if err != nil {
		return nil, fmt.Errorf("failed to get peer devices: %w", err)
	}
	if len(devices) == 0 {
		devices = []types.JID{peerLID}
	}

	routeJID := devices[0]
	for _, device := range devices {
		if device.Device == 0 {
			routeJID = device
			break
		}
	}

	routeHadSession, err := p.backend.HasSession(ctx, routeJID)
	if err != nil {
		return nil, fmt.Errorf("failed to check signal session for %s: %w", routeJID, err)
	}

	if err = p.backend.EnsureSignalSessions(ctx, devices); err != nil {
		return nil, fmt.Errorf("failed to establish signal sessions: %w", err)
	}

	if !routeHadSession {
		return nil, fmt.Errorf(
			"no established signal session with %s (%s); send a message to this contact before calling",
			peerPN, routeJID,
		)
	}

	tcToken, err := p.ensureTCToken(ctx, peerLID, peerPN)
	if err != nil {
		return nil, err
	}

	return &PeerInfo{
		PeerPN:      peerPN,
		PeerLID:     peerLID,
		RouteJID:    routeJID,
		PeerDevices: devices,
		TCToken:     tcToken,
	}, nil
}

func (p *PeerPreparer) ensureTCToken(ctx context.Context, peerLID, peerPN types.JID) ([]byte, error) {
	for _, jid := range []types.JID{peerLID, peerPN} {
		token, err := p.backend.EnsureTCToken(ctx, jid)
		if err != nil {
			return nil, err
		}
		if len(token) > 0 {
			return token, nil
		}
	}
	for _, jid := range []types.JID{peerLID, peerPN} {
		if err := p.backend.IssuePrivacyToken(ctx, jid); err != nil {
			p.backend.LogWarnf("Failed to issue privacy token for %s: %v", jid, err)
			continue
		}
		token, err := p.backend.EnsureTCToken(ctx, jid)
		if err != nil {
			return nil, err
		}
		if len(token) > 0 {
			return token, nil
		}
	}
	return nil, nil
}

func waitContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
