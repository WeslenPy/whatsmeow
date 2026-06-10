// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"crypto/rand"
	"fmt"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// WebCapabilityBytes is the capability blob used by WhatsApp Web linked devices.
var WebCapabilityBytes = []byte{1, 4, 247, 11, 206, 3}

// DefaultVoIPSettingsJSON is a minimal voip_settings stub until WASM provides the full blob.
var DefaultVoIPSettingsJSON = []byte(`{"encode":{"codec":"opus","complexity":"5","frame_ms":"60","min_bitrate":"4200"},"options":{"disable_p2p":"1","caller_timeout":"90"},"voip_settings_version":{"release_type":"1","version_number":"151059"}}`)

// Offer child order from captured inbound Android offer (2026-06-10).
var offerChildOrder = []string{
	"audio", "audio", "capability", "enc", "encopt", "metadata", "net",
	"rte", "uploadfieldstat", "voip_settings", "relay",
}

// OfferParams contains inputs for building an outbound call offer node.
type OfferParams struct {
	CallID      string
	CallCreator types.JID
	CallerPN    types.JID
	RouteJID    types.JID

	IsVideo            bool
	Joinable           bool
	CallerCountryCode  string
	DeviceClass        string
	Capability         []byte
	VoIPSettings       []byte
	RTE                []byte // optional 6-byte IPv4:port transport hint
	Relay              *waBinary.Node
}

// OfferBuilder constructs outbound call offer nodes.
// Raw callKey in the enc child is replaced with encrypted content by SignalingBridge.
type OfferBuilder struct{}

func NewOfferBuilder() *OfferBuilder {
	return &OfferBuilder{}
}

// Build creates the inner offer node before E2E encryption of callKey.
// Structure mirrors real inbound offers: single root enc, no destination.
func (b *OfferBuilder) Build(params OfferParams) (*waBinary.Node, []byte, error) {
	if params.CallID == "" {
		return nil, nil, fmt.Errorf("call ID is required")
	} else if params.CallCreator.IsEmpty() {
		return nil, nil, fmt.Errorf("call creator is required")
	} else if params.RouteJID.IsEmpty() {
		return nil, nil, fmt.Errorf("route JID is required")
	}

	callKey, err := generateCallKey()
	if err != nil {
		return nil, nil, err
	}

	attrs := waBinary.Attrs{
		"call-id":      params.CallID,
		"call-creator": params.CallCreator,
		"joinable":     "1",
	}
	if !params.CallerPN.IsEmpty() {
		attrs["caller_pn"] = params.CallerPN
	}
	if params.CallerCountryCode != "" {
		attrs["caller_country_code"] = params.CallerCountryCode
	}
	if params.DeviceClass != "" {
		attrs["device_class"] = params.DeviceClass
	} else {
		attrs["device_class"] = "2016"
	}

	capability := params.Capability
	if len(capability) == 0 {
		capability = WebCapabilityBytes
	}
	voipSettings := params.VoIPSettings
	if len(voipSettings) == 0 {
		voipSettings = DefaultVoIPSettingsJSON
	}

	children := []waBinary.Node{
		{Tag: "audio", Attrs: waBinary.Attrs{"enc": "opus", "rate": "16000"}},
		{Tag: "audio", Attrs: waBinary.Attrs{"enc": "opus", "rate": "8000"}},
	}
	if params.IsVideo {
		children = append(children, waBinary.Node{
			Tag: "video",
			Attrs: waBinary.Attrs{
				"orientation":        "0",
				"screen_width":       "1080",
				"screen_height":      "2340",
				"device_orientation": "0",
				"enc":                "vp8",
				"dec":                "vp8",
			},
		})
	}

	children = append(children,
		waBinary.Node{
			Tag:     "capability",
			Attrs:   waBinary.Attrs{"ver": "1"},
			Content: append([]byte(nil), capability...),
		},
		waBinary.Node{
			Tag:     "enc",
			Attrs:   waBinary.Attrs{"v": "2", "count": "0"},
			Content: append([]byte(nil), callKey...),
		},
		waBinary.Node{Tag: "encopt", Attrs: waBinary.Attrs{"keygen": "2"}},
		waBinary.Node{
			Tag: "metadata",
			Attrs: waBinary.Attrs{
				"peer_abtest_bucket":            "",
				"peer_abtest_bucket_id_list":    "",
			},
		},
		waBinary.Node{Tag: "net", Attrs: waBinary.Attrs{"medium": "3"}},
	)

	if len(params.RTE) > 0 {
		children = append(children, waBinary.Node{
			Tag:     "rte",
			Content: append([]byte(nil), params.RTE...),
		})
	}

	children = append(children,
		waBinary.Node{Tag: "uploadfieldstat"},
		waBinary.Node{
			Tag:     "voip_settings",
			Attrs:   waBinary.Attrs{"uncompressed": "1"},
			Content: append([]byte(nil), voipSettings...),
		},
	)

	if params.Relay != nil {
		children = append(children, *params.Relay)
	}

	return &waBinary.Node{Tag: "offer", Attrs: attrs, Content: children}, callKey, nil
}

// OfferChildIndex returns the index of a child tag in the canonical offer order (-1 if absent).
func OfferChildIndex(tag string) int {
	for i, t := range offerChildOrder {
		if t == tag {
			return i
		}
	}
	return -1
}

func generateCallKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate call key: %w", err)
	}
	return key, nil
}
