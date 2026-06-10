// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"encoding/base64"
	"fmt"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// RelayEndpoint is a single edge relay (te2 child inside relay).
type RelayEndpoint struct {
	RelayID     string
	RelayName   string
	DomainName  string
	TokenID     string
	AuthTokenID string
	C2RRTT      string
	IsFNA       bool
	// IPPort is 6 bytes (IPv4+port BE) or 18 bytes (IPv6+port BE).
	IPPort []byte
}

// RelayOfferInfo contains relay subtree data for an outbound offer.
// Tokens and auth material are allocated by WhatsApp servers (VoIP WASM).
type RelayOfferInfo struct {
	UUID             string
	PeerPID          string
	SelfPID          string
	AttributePadding bool
	ParticipantJID   types.JID
	ParticipantPID   string
	Tokens           map[string][]byte
	AuthTokens       map[string][]byte
	Key              []byte // raw base64 ASCII of inner key, stored as bytes
	HBHKey           []byte // raw ASCII for STUN HMAC
	Endpoints        []RelayEndpoint
}

// BuildRelayNode constructs a relay child matching captured inbound structure.
func BuildRelayNode(info RelayOfferInfo) (*waBinary.Node, error) {
	if info.UUID == "" {
		return nil, fmt.Errorf("relay UUID is required")
	}
	attrs := waBinary.Attrs{
		"uuid":     info.UUID,
		"peer_pid": info.PeerPID,
		"self_pid": info.SelfPID,
	}
	if info.AttributePadding {
		attrs["attribute_padding"] = "1"
	}

	var children []waBinary.Node
	if !info.ParticipantJID.IsEmpty() {
		children = append(children, waBinary.Node{
			Tag: "participant",
			Attrs: waBinary.Attrs{
				"jid": info.ParticipantJID,
				"pid": info.ParticipantPID,
			},
		})
	}
	for id, tok := range info.Tokens {
		children = append(children, waBinary.Node{
			Tag:     "token",
			Attrs:   waBinary.Attrs{"id": id},
			Content: append([]byte(nil), tok...),
		})
	}
	for id, tok := range info.AuthTokens {
		children = append(children, waBinary.Node{
			Tag:     "auth_token",
			Attrs:   waBinary.Attrs{"id": id},
			Content: append([]byte(nil), tok...),
		})
	}
	if len(info.Key) > 0 {
		children = append(children, waBinary.Node{
			Tag:     "key",
			Content: append([]byte(nil), info.Key...),
		})
	}
	for _, ep := range info.Endpoints {
		if len(ep.IPPort) == 0 {
			continue
		}
		epAttrs := waBinary.Attrs{
			"relay_id":      ep.RelayID,
			"relay_name":    ep.RelayName,
			"token_id":      ep.TokenID,
			"auth_token_id": ep.AuthTokenID,
		}
		if ep.C2RRTT != "" {
			epAttrs["c2r_rtt"] = ep.C2RRTT
		}
		if ep.DomainName != "" {
			epAttrs["domain_name"] = ep.DomainName
		}
		if ep.IsFNA {
			epAttrs["is_fna"] = "1"
		}
		children = append(children, waBinary.Node{
			Tag:     "te2",
			Attrs:   epAttrs,
			Content: append([]byte(nil), ep.IPPort...),
		})
	}
	if len(info.HBHKey) > 0 {
		children = append(children, waBinary.Node{
			Tag:     "hbh_key",
			Content: append([]byte(nil), info.HBHKey...),
		})
	}
	if len(children) == 0 {
		return nil, fmt.Errorf("relay node has no children")
	}
	return &waBinary.Node{Tag: "relay", Attrs: attrs, Content: children}, nil
}

// DecodeRelayKey decodes the double-base64 relay key from a key node payload.
func DecodeRelayKey(keyContent []byte) ([]byte, error) {
	inner, err := base64.StdEncoding.DecodeString(string(keyContent))
	if err != nil {
		inner, err = base64.RawStdEncoding.DecodeString(string(keyContent))
		if err != nil {
			return keyContent, nil
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(string(inner))
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(string(inner))
		if err != nil {
			return inner, nil
		}
	}
	return decoded, nil
}
