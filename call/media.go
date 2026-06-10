// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

// Media integration (Phase 7 — future work):
//
// - VoIP WASM via Node sidecar only (cmd/voip-bridge)
// - Relay transport in Go: call/stun.go, call/relay_transport.go, call/relay_handler.go
// - Relay transport using pion/webrtc with synthetic SDP (see baileys-caller relay-transport.mts)
// - Audio pipeline: PCM 16kHz mono, 320-frame chunks; Opus encode/decode inside WASM
// - IPC between Go signaling bridge and WASM sidecar for transport/relay packets
//
// The current package implements signaling only (offer, preaccept, accept, terminate).
