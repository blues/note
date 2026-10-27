// Copyright 2026 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"crypto/tls"
	"net"
	"sync/atomic"
	"time"

	"github.com/blues/note-go/note"
)

// CountingConn wraps a net.Conn and counts the raw bytes that cross it.  It is placed
// under TLS so that handshake and record overhead are counted.
type CountingConn struct {
	net.Conn
	read, written atomic.Uint64
}

// Read counts every byte returned, including a partial count returned with an error
func (c *CountingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.read.Add(uint64(n))
	return n, err
}

// Write counts every byte written, including a partial count returned with an error
func (c *CountingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.written.Add(uint64(n))
	return n, err
}

// BytesRead is the number of bytes read from the device so far
func (c *CountingConn) BytesRead() uint64 {
	return c.read.Load()
}

// BytesWritten is the number of bytes written to the device so far
func (c *CountingConn) BytesWritten() uint64 {
	return c.written.Load()
}

// FindCountingConn returns the CountingConn underneath conn, looking through TLS,
// or nil if conn isn't counted
func FindCountingConn(conn net.Conn) *CountingConn {
	if tlsConn, ok := conn.(*tls.Conn); ok {
		conn = tlsConn.NetConn()
	}
	cc, _ := conn.(*CountingConn)
	return cc
}

// UpdateWireUsage copies the socket byte counts into the session's packet usage, under
// TcpPs.  Direction is from the device's point of view: uplink = device to Notehub.  It does
// nothing for sessions that have no counted connection.
func (session *HubSession) UpdateWireUsage() {
	if session.WireConn == nil {
		return
	}
	if session.Session.PacketUsage == nil {
		session.Session.PacketUsage = map[string]note.PacketUsage{}
	}
	u := session.Session.PacketUsage[TcpPs]
	u.Updated = time.Now().Unix()
	u.UplinkBytes = int64(session.WireConn.BytesRead())
	u.DownlinkBytes = int64(session.WireConn.BytesWritten())
	session.Session.PacketUsage[TcpPs] = u
}
