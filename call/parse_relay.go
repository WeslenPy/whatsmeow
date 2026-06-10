// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"encoding/binary"
	"fmt"
	"net"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// ParsedRelay is decoded relay info from an offer or accept node.
type ParsedRelay struct {
	UUID       string
	PeerPID    string
	SelfPID    string
	Participant types.JID
	Tokens     map[string][]byte
	AuthTokens map[string][]byte
	KeyRaw     []byte
	Key        []byte
	HBHKeyRaw  []byte
	Endpoints  []ParsedRelayEndpoint
}

// ParsedRelayEndpoint is a single te2 edge relay endpoint.
type ParsedRelayEndpoint struct {
	RelayID     string
	RelayName   string
	DomainName  string
	TokenID     string
	AuthTokenID string
	C2RRTT      string
	IsFNA       bool
	IP          net.IP
	Port        uint16
}

// ParseRelayNode extracts relay server info from a relay child node.
func ParseRelayNode(node *waBinary.Node) (*ParsedRelay, error) {
	if node == nil || node.Tag != "relay" {
		return nil, fmt.Errorf("expected relay node, got %q", nodeTag(node))
	}
	ag := node.AttrGetter()
	relay := &ParsedRelay{
		UUID:       ag.String("uuid"),
		PeerPID:    ag.String("peer_pid"),
		SelfPID:    ag.String("self_pid"),
		Tokens:     make(map[string][]byte),
		AuthTokens: make(map[string][]byte),
	}
	if !ag.OK() {
		return nil, fmt.Errorf("invalid relay attributes")
	}

	for _, child := range node.GetChildren() {
		switch child.Tag {
		case "participant":
			relay.Participant = child.AttrGetter().JID("jid")
		case "token":
			id := child.AttrGetter().String("id")
			if data := nodeBytes(child); data != nil {
				relay.Tokens[id] = data
			}
		case "auth_token":
			id := child.AttrGetter().String("id")
			if data := nodeBytes(child); data != nil {
				relay.AuthTokens[id] = data
			}
		case "key":
			relay.KeyRaw = nodeBytes(child)
			relay.Key, _ = DecodeRelayKey(relay.KeyRaw)
		case "hbh_key":
			relay.HBHKeyRaw = nodeBytes(child)
		case "te2":
			if ep := parseTE2Endpoint(child); ep != nil {
				relay.Endpoints = append(relay.Endpoints, *ep)
			}
		}
	}
	if len(relay.Endpoints) == 0 {
		return nil, fmt.Errorf("relay has no endpoints")
	}
	return relay, nil
}

func parseTE2Endpoint(node waBinary.Node) *ParsedRelayEndpoint {
	data := nodeBytes(node)
	if data == nil {
		return nil
	}
	ag := node.AttrGetter()
	ep := &ParsedRelayEndpoint{
		RelayID:     ag.String("relay_id"),
		RelayName:   ag.String("relay_name"),
		DomainName:  ag.String("domain_name"),
		TokenID:     ag.String("token_id"),
		AuthTokenID: ag.String("auth_token_id"),
		C2RRTT:      ag.String("c2r_rtt"),
		IsFNA:       ag.String("is_fna") == "1",
	}
	switch len(data) {
	case 6:
		ep.IP = net.IP(append([]byte(nil), data[:4]...))
		ep.Port = binary.BigEndian.Uint16(data[4:6])
	case 18:
		ep.IP = net.IP(append([]byte(nil), data[:16]...))
		ep.Port = binary.BigEndian.Uint16(data[16:18])
	default:
		return nil
	}
	return ep
}

func nodeBytes(node waBinary.Node) []byte {
	if node.Content == nil {
		return nil
	}
	switch v := node.Content.(type) {
	case []byte:
		return append([]byte(nil), v...)
	case string:
		return []byte(v)
	default:
		return nil
	}
}

func nodeTag(node *waBinary.Node) string {
	if node == nil {
		return ""
	}
	return node.Tag
}
