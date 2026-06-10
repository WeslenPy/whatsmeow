// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"sync"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// OutboundState tracks the lifecycle of an outbound call session.
type OutboundState int

const (
	StateOffering OutboundState = iota
	StateRinging
	StateSignalingConnected
	StateEnded
)

func (s OutboundState) String() string {
	switch s {
	case StateOffering:
		return "offering"
	case StateRinging:
		return "ringing"
	case StateSignalingConnected:
		return "signaling_connected"
	case StateEnded:
		return "ended"
	default:
		return "unknown"
	}
}

// OutboundSession holds state for a single outbound call.
type OutboundSession struct {
	CallID      string
	PeerPN      types.JID
	PeerLID     types.JID
	RouteJID    types.JID
	IsVideo     bool
	State       OutboundState
	EndReason   string
	CreatedAt   time.Time
	ConnectedAt time.Time

	// Captured signaling nodes for protocol analysis / future media bridge.
	CapturedNodes []CapturedNode
	onEnd         func(reason string)

	mu sync.RWMutex
}

type CapturedNode struct {
	Direction string // "inbound" or "outbound"
	Tag       string
	Node      *waBinary.Node
	Timestamp time.Time
}

func (s *OutboundSession) setState(state OutboundState) {
	s.mu.Lock()
	s.State = state
	s.mu.Unlock()
}

func (s *OutboundSession) getState() OutboundState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *OutboundSession) capture(direction, tag string, node *waBinary.Node) {
	if node == nil {
		return
	}
	cloned := cloneNode(node)
	s.mu.Lock()
	s.CapturedNodes = append(s.CapturedNodes, CapturedNode{
		Direction: direction,
		Tag:       tag,
		Node:      &cloned,
		Timestamp: time.Now(),
	})
	s.mu.Unlock()
}

func cloneNode(node *waBinary.Node) waBinary.Node {
	out := waBinary.Node{
		Tag:   node.Tag,
		Attrs: make(waBinary.Attrs, len(node.Attrs)),
	}
	for k, v := range node.Attrs {
		out.Attrs[k] = v
	}
	switch content := node.Content.(type) {
	case []byte:
		out.Content = append([]byte(nil), content...)
	case []waBinary.Node:
		children := make([]waBinary.Node, len(content))
		for i := range content {
			children[i] = cloneNode(&content[i])
		}
		out.Content = children
	default:
		out.Content = content
	}
	return out
}
