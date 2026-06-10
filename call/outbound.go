// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"context"
	"sync"

	"go.mau.fi/whatsmeow/types"
)

// OutboundCall is a handle to an active outbound call session.
type OutboundCall struct {
	manager *Manager
	session *OutboundSession

	timeoutCancel context.CancelFunc
	endOnce       sync.Once
	endCh         chan string
}

// CallID returns the WhatsApp call identifier.
func (c *OutboundCall) CallID() string {
	return c.session.CallID
}

// PeerJID returns the peer phone number JID.
func (c *OutboundCall) PeerJID() types.JID {
	return c.session.PeerPN
}

// State returns the current outbound call state.
func (c *OutboundCall) State() OutboundState {
	return c.session.getState()
}

// Cancel terminates the outbound call.
func (c *OutboundCall) Cancel(ctx context.Context) error {
	return c.manager.cancelSession(ctx, c.session, "cancelled")
}

// Wait blocks until the call ends and returns the end reason.
func (c *OutboundCall) Wait(ctx context.Context) (string, error) {
	select {
	case reason := <-c.endCh:
		return reason, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *OutboundCall) startRingingTimeout(ctx context.Context) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.manager.opts.RingingTimeout)
	c.timeoutCancel = cancel
	go func() {
		<-timeoutCtx.Done()
		if timeoutCtx.Err() == context.DeadlineExceeded {
			_ = c.manager.cancelSession(context.Background(), c.session, "timeout")
		}
	}()
}

func (c *OutboundCall) notifyEnded(reason string) {
	c.endOnce.Do(func() {
		if c.timeoutCancel != nil {
			c.timeoutCancel()
		}
		c.endCh <- reason
		close(c.endCh)
	})
}
