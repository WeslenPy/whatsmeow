// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// RelayTransport sends and receives UDP packets to WhatsApp edge relays.
type RelayTransport struct {
	mu      sync.Mutex
	conns   map[string]*net.UDPConn
	timeout time.Duration
	onRecv  func(data []byte, ip string, port uint16)
	logf    func(msg string, args ...any)
}

// RelayTransportConfig configures the relay UDP transport.
type RelayTransportConfig struct {
	Timeout time.Duration
	OnRecv  func(data []byte, ip string, port uint16)
	Logf    func(msg string, args ...any)
}

// NewRelayTransport creates a UDP transport for relay edge servers.
func NewRelayTransport(cfg RelayTransportConfig) *RelayTransport {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	rt := &RelayTransport{
		conns:   make(map[string]*net.UDPConn),
		timeout: timeout,
		onRecv:  cfg.OnRecv,
		logf:    cfg.Logf,
	}
	if rt.logf == nil {
		rt.logf = func(string, ...any) {}
	}
	return rt
}

// SendDataToRelay sends a UDP packet to the relay at ip:port.
func (rt *RelayTransport) SendDataToRelay(data []byte, ip string, port uint16) error {
	if len(data) == 0 {
		return fmt.Errorf("empty relay packet")
	}
	addr := &net.UDPAddr{IP: net.ParseIP(ip), Port: int(port)}
	if addr.IP == nil {
		return fmt.Errorf("invalid relay IP %q", ip)
	}
	conn, err := rt.getConn(addr)
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP(data, addr)
	return err
}

// Close shuts down all relay UDP sockets.
func (rt *RelayTransport) Close() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var firstErr error
	for key, conn := range rt.conns {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(rt.conns, key)
	}
	return firstErr
}

func (rt *RelayTransport) getConn(localHint *net.UDPAddr) (*net.UDPConn, error) {
	key := "default"
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if conn, ok := rt.conns[key]; ok {
		return conn, nil
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("listen relay UDP: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(rt.timeout))
	rt.conns[key] = conn
	go rt.readLoop(conn)
	return conn, nil
}

func (rt *RelayTransport) readLoop(conn *net.UDPConn) {
	buf := make([]byte, 4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(rt.timeout))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		if rt.onRecv != nil && addr != nil {
			data := append([]byte(nil), buf[:n]...)
			rt.onRecv(data, addr.IP.String(), uint16(addr.Port))
		}
	}
}
