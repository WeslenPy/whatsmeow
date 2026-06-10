// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package sidecar implements JSON-lines IPC with the Node.js VoIP WASM bridge.
package sidecar

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"go.mau.fi/whatsmeow/types"
)

// Config configures the Node sidecar subprocess.
type Config struct {
	NodeCommand string
	BridgeScript string
	RepoRoot     string
	OnSignalingXmpp func(peerJID types.JID, callID string, payload []byte)
	OnCallEvent     func(callID string, eventType int, eventData string)
	SendDataToRelay func(data []byte, ip string, port uint16) error
	LogDebug        func(msg string, args ...any)
	LogWarn         func(msg string, args ...any)
}

// Client talks to cmd/voip-bridge/bridge.mjs over stdin/stdout.
type Client struct {
	cfg    Config
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	nextID atomic.Uint64
	ready  chan struct{}
	closed bool

	pendingMu sync.Mutex
	pending   map[uint64]chan rpcLine
}

type rpcRequest struct {
	ID   uint64 `json:"id"`
	Op   string `json:"op"`
	Data map[string]any
}

type rpcLine struct {
	ID        uint64 `json:"id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
	Event     string `json:"event"`
	PeerJID   string `json:"peerJid"`
	CallID    string `json:"callId"`
	Payload   string `json:"payload"`
	IP        string `json:"ip"`
	Port      uint16 `json:"port"`
	Data      string `json:"data"`
	EventType int    `json:"eventType"`
	EventData string `json:"eventData"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RepoRoot  string `json:"repoRoot"`
	WasmPath  string `json:"wasmPath"`
	Debug     bool   `json:"debug"`
}

func bridgeDebugEnabled() bool {
	return os.Getenv("VOIP_BRIDGE_DEBUG") == "1"
}

// Start launches the Node bridge subprocess.
func Start(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.NodeCommand == "" {
		cfg.NodeCommand = DefaultNodeCommand()
	}
	if cfg.BridgeScript == "" {
		cfg.BridgeScript = DefaultBridgeScript()
	}
	if cfg.RepoRoot == "" {
		cfg.RepoRoot = findModuleRoot()
	}

	c := &Client{cfg: cfg, ready: make(chan struct{}), pending: make(map[uint64]chan rpcLine)}
	c.cmd = exec.CommandContext(ctx, cfg.NodeCommand, cfg.BridgeScript)
	env := append(os.Environ(), "WASM_RESOURCES_PATH="+cfg.RepoRoot)
	if bridgeDebugEnabled() {
		env = append(env, "VOIP_BRIDGE_DEBUG=1")
	}
	c.cmd.Env = env
	if cfg.RepoRoot != "" {
		c.cmd.Dir = cfg.RepoRoot
	}
	c.cmd.Stderr = os.Stderr

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.stdin = stdin

	if err := c.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start voip bridge: %w", err)
	}

	go c.readLoop(stdout)
	select {
	case <-c.ready:
	case <-ctx.Done():
		_ = c.Close()
		return nil, ctx.Err()
	}
	return c, nil
}

func (c *Client) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		var line rpcLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			c.logWarn("sidecar parse error: %v", err)
			continue
		}
		if line.Event == "ready" {
			c.logDebug("sidecar ready repo=%s wasm=%s bridgeDebug=%v", line.RepoRoot, line.WasmPath, line.Debug)
			close(c.ready)
			continue
		}
		if line.Event != "" {
			c.dispatchEvent(line)
			continue
		}
		if line.ID != 0 {
			c.pendingMu.Lock()
			ch := c.pending[line.ID]
			c.pendingMu.Unlock()
			if ch != nil {
				ch <- line
				continue
			}
			if !line.OK && line.Error != "" {
				c.logWarn("sidecar rpc %d failed: %s", line.ID, line.Error)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		c.logWarn("sidecar read error: %v", err)
	}
}

func (c *Client) dispatchEvent(line rpcLine) {
	switch line.Event {
	case "onSignalingXmpp":
		c.logDebug("sidecar event onSignalingXmpp peer=%s call=%s payloadB64=%d", line.PeerJID, line.CallID, len(line.Payload))
		if c.cfg.OnSignalingXmpp == nil {
			return
		}
		jid, _ := types.ParseJID(line.PeerJID)
		payload, _ := base64.StdEncoding.DecodeString(line.Payload)
		c.cfg.OnSignalingXmpp(jid, line.CallID, payload)
	case "sendDataToRelay":
		c.logDebug("sidecar event sendDataToRelay ip=%s port=%d dataB64=%d", line.IP, line.Port, len(line.Data))
		if c.cfg.SendDataToRelay == nil {
			return
		}
		data, _ := base64.StdEncoding.DecodeString(line.Data)
		_ = c.cfg.SendDataToRelay(data, line.IP, line.Port)
	case "onCallEvent":
		c.logDebug("sidecar event onCallEvent type=%d dataLen=%d", line.EventType, len(line.EventData))
		if c.cfg.OnCallEvent == nil {
			return
		}
		c.cfg.OnCallEvent("", line.EventType, line.EventData)
	case "log":
		if line.Level == "warn" {
			c.logWarn("wasm: %s", line.Message)
		} else {
			c.logDebug("wasm: %s", line.Message)
		}
	}
}

func (c *Client) call(ctx context.Context, op string, fields map[string]any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("sidecar closed")
	}
	id := c.nextID.Add(1)
	respCh := make(chan rpcLine, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	req := map[string]any{"id": id, "op": op}
	for k, v := range fields {
		req[k] = v
	}
	data, err := json.Marshal(req)
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		c.mu.Unlock()
		return err
	}
	data = append(data, '\n')
	c.logDebug("sidecar rpc -> id=%d op=%s bytes=%d", id, op, len(data))
	_, err = c.stdin.Write(data)
	c.mu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return fmt.Errorf("sidecar write: %w", err)
	}

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	select {
	case resp := <-respCh:
		if !resp.OK {
			if resp.Error != "" {
				c.logWarn("sidecar rpc <- id=%d op=%s ok=false err=%s", id, op, resp.Error)
				return fmt.Errorf("%s", resp.Error)
			}
			c.logWarn("sidecar rpc <- id=%d op=%s ok=false", id, op)
			return fmt.Errorf("sidecar rpc %d failed", id)
		}
		c.logDebug("sidecar rpc <- id=%d op=%s ok=true", id, op)
		return nil
	case <-ctx.Done():
		c.logWarn("sidecar rpc <- id=%d op=%s ctx=%v", id, op, ctx.Err())
		return ctx.Err()
	}
}

// Init initializes the WASM stack with own JIDs.
func (c *Client) Init(ctx context.Context, ownPN, ownLID types.JID) error {
	return c.call(ctx, "init", map[string]any{
		"ownPN":  ownPN.String(),
		"ownLID": ownLID.String(),
	})
}

// StartCall starts an outbound VoIP call in the WASM engine.
func (c *Client) StartCall(ctx context.Context, params StartCallParams) error {
	peerList := make([]string, len(params.PeerList))
	for i, j := range params.PeerList {
		peerList[i] = j.String()
	}
	return c.call(ctx, "startCall", map[string]any{
		"callId":    params.CallID,
		"peerJid":   params.PeerJID.String(),
		"peerPn":    params.PeerPN.String(),
		"peerList":  peerList,
		"isVideo":   params.IsVideo,
		"isLidCall": params.IsLIDCall,
		"tcToken":   base64.StdEncoding.EncodeToString(params.TCToken),
	})
}

// EndCall terminates the WASM call session.
func (c *Client) EndCall(callID string, reason int, sendTerminate bool) error {
	return c.call(context.Background(), "endCall", map[string]any{
		"callId":        callID,
		"reason":        reason,
		"sendTerminate": sendTerminate,
	})
}

// HandleSignalingAck forwards a server ack to WASM.
func (c *Client) HandleSignalingAck(ctx context.Context, params SignalingAckParams) error {
	return c.call(ctx, "handleSignalingAck", map[string]any{
		"payload":  params.Payload,
		"ackError": params.AckError,
		"msgType":  params.MsgType,
		"peerJid":  params.PeerJID.String(),
		"tcToken":  base64.StdEncoding.EncodeToString(params.TCToken),
	})
}

// HandleSignalingMessage forwards an inbound signaling stanza to WASM.
func (c *Client) HandleSignalingMessage(ctx context.Context, params SignalingMessageParams) error {
	return c.call(ctx, "handleSignalingMessage", map[string]any{
		"payload":        params.Payload,
		"peerJid":        params.PeerJID.String(),
		"peerPlatform":   params.PeerPlatform,
		"peerAppVersion": params.PeerAppVersion,
		"epochId":        params.EpochID,
		"timestamp":      params.Timestamp,
		"isOffline":      params.IsOffline,
		"tcToken":        base64.StdEncoding.EncodeToString(params.TCToken),
	})
}

// HandleSignalingOffer forwards an inbound offer to WASM.
func (c *Client) HandleSignalingOffer(ctx context.Context, params SignalingMessageParams) error {
	return c.call(ctx, "handleSignalingOffer", map[string]any{
		"payload":        params.Payload,
		"peerJid":        params.PeerJID.String(),
		"peerPlatform":   params.PeerPlatform,
		"peerAppVersion": params.PeerAppVersion,
		"epochId":        params.EpochID,
		"timestamp":      params.Timestamp,
		"isOffline":      params.IsOffline,
		"tcToken":        base64.StdEncoding.EncodeToString(params.TCToken),
	})
}

// HandleSignalingReceipt forwards a call receipt to WASM.
func (c *Client) HandleSignalingReceipt(ctx context.Context, params SignalingMessageParams) error {
	return c.call(ctx, "handleSignalingReceipt", map[string]any{
		"payload": params.Payload,
		"peerJid": params.PeerJID.String(),
		"tcToken": base64.StdEncoding.EncodeToString(params.TCToken),
	})
}

// HandleTransportMessage forwards relay UDP data to WASM.
func (c *Client) HandleTransportMessage(data []byte, ip string, port uint16) error {
	return c.call(context.Background(), "handleTransportMessage", map[string]any{
		"data": base64.StdEncoding.EncodeToString(data),
		"ip":   ip,
		"port": port,
	})
}

// Close shuts down the sidecar subprocess.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	_ = c.call(context.Background(), "shutdown", nil)
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}

func (c *Client) logDebug(msg string, args ...any) {
	if !bridgeDebugEnabled() {
		return
	}
	if c.cfg.LogDebug != nil {
		c.cfg.LogDebug(msg, args...)
	}
}

func (c *Client) logWarn(msg string, args ...any) {
	if c.cfg.LogWarn != nil {
		c.cfg.LogWarn(msg, args...)
	}
}

// StartCallParams mirrors call/wasm.StartCallParams.
type StartCallParams struct {
	PeerJID   types.JID
	PeerPN    types.JID
	CallID    string
	IsVideo   bool
	IsLIDCall bool
	TCToken   []byte
	PeerList  []types.JID
}

// SignalingAckParams mirrors call/wasm.SignalingAckParams.
type SignalingAckParams struct {
	Payload  string
	AckError string
	MsgType  string
	PeerJID  types.JID
	TCToken  []byte
}

// SignalingMessageParams mirrors call/wasm.SignalingMessageParams.
type SignalingMessageParams struct {
	Payload        string
	PeerJID        types.JID
	PeerPlatform   string
	PeerAppVersion string
	EpochID        string
	Timestamp      string
	IsOffline      bool
	TCToken        []byte
}
