package notelib

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blues/note-go/note"
	"github.com/stretchr/testify/require"
)

func selfSignedTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return &tls.Config{
		Certificates:           []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		SessionTicketsDisabled: true,
	}
}

func TestCountingConnCountsBothDirections(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	cc := &CountingConn{Conn: server}

	go func() {
		_, _ = client.Write(make([]byte, 10))
		_, _ = io.ReadFull(client, make([]byte, 7))
	}()

	// Read in two partial reads to prove every Read call is counted
	_, err := io.ReadFull(cc, make([]byte, 4))
	require.NoError(t, err)
	_, err = io.ReadFull(cc, make([]byte, 6))
	require.NoError(t, err)
	_, err = cc.Write(make([]byte, 7))
	require.NoError(t, err)

	require.Equal(t, uint64(10), cc.BytesRead())
	require.Equal(t, uint64(7), cc.BytesWritten())
}

// errConn returns a partial count together with an error, as a socket does on disconnect.
type errConn struct{ net.Conn }

func (errConn) Read(b []byte) (int, error)  { return 3, io.ErrUnexpectedEOF }
func (errConn) Write(b []byte) (int, error) { return 2, io.ErrShortWrite }

func TestCountingConnCountsPartialIOWithError(t *testing.T) {
	cc := &CountingConn{Conn: errConn{}}

	n, err := cc.Read(make([]byte, 8))
	require.Equal(t, 3, n)
	require.True(t, errors.Is(err, io.ErrUnexpectedEOF))

	n, err = cc.Write(make([]byte, 8))
	require.Equal(t, 2, n)
	require.True(t, errors.Is(err, io.ErrShortWrite))

	require.Equal(t, uint64(3), cc.BytesRead())
	require.Equal(t, uint64(2), cc.BytesWritten())
}

func TestCountingConnUnderTLSCountsHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	payload := []byte("hello notehub")
	clientErr := make(chan error, 1)
	go func() {
		c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			clientErr <- err
			return
		}
		defer c.Close()
		_, err = c.Write(payload)
		clientErr <- err
	}()

	raw, err := ln.Accept()
	require.NoError(t, err)
	cc := &CountingConn{Conn: raw}
	srv := tls.Server(cc, selfSignedTLSConfig(t))
	defer srv.Close()

	got := make([]byte, len(payload))
	_, err = io.ReadFull(srv, got)
	require.NoError(t, err)
	require.NoError(t, <-clientErr)
	require.Equal(t, payload, got)

	// The socket saw the handshake and record overhead, not just the payload
	require.Greater(t, cc.BytesRead(), uint64(len(payload)))
	require.Greater(t, cc.BytesWritten(), uint64(0))

	require.Same(t, cc, FindCountingConn(srv))
}

func TestFindCountingConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cc := &CountingConn{Conn: server}
	require.Same(t, cc, FindCountingConn(cc), "plain TCP: the wrapper itself")
	require.Nil(t, FindCountingConn(server), "unwrapped conn")
	require.Nil(t, FindCountingConn(tls.Server(server, &tls.Config{})), "TLS over an unwrapped conn")
}

func TestCountingConnConcurrentAccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	cc := &CountingConn{Conn: server}

	go func() { _, _ = io.Copy(io.Discard, client) }()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = cc.Write([]byte{1})
		}
	}()
	go func() {
		defer wg.Done()
		s := HubSession{WireConn: cc}
		for i := 0; i < 100; i++ {
			s.UpdateWireUsage()
		}
	}()
	wg.Wait()
	require.Equal(t, uint64(100), cc.BytesWritten())
}

func TestUpdateWireUsageWithoutConnLeavesSessionUntouched(t *testing.T) {
	s := HubSession{}
	s.UpdateWireUsage()
	require.Nil(t, s.Session.PacketUsage, "sessions with no WireConn must not gain packet_usage")
}

func TestUpdateWireUsageSetsTcpPacketUsage(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	cc := &CountingConn{Conn: server}
	go func() {
		_, _ = client.Write(make([]byte, 5))
		_, _ = io.ReadFull(client, make([]byte, 3))
	}()
	_, err := io.ReadFull(cc, make([]byte, 5))
	require.NoError(t, err)
	_, err = cc.Write(make([]byte, 3))
	require.NoError(t, err)

	s := HubSession{WireConn: cc}
	s.Session.PacketUsage = map[string]note.PacketUsage{"skylo:1": {UplinkBytes: 99}}
	s.UpdateWireUsage()
	s.UpdateWireUsage() // counts are cumulative, so repeated copies must not add up

	u := s.Session.PacketUsage[TcpPs]
	require.Equal(t, int64(5), u.UplinkBytes)
	require.Equal(t, int64(3), u.DownlinkBytes)
	require.NotZero(t, u.Updated)
	require.Equal(t, note.PacketUsage{UplinkBytes: 99}, s.Session.PacketUsage["skylo:1"], "other keys are left alone")
}
