// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Inbound TCP support
package main

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	notelib "github.com/blues/note/lib"
)

// tcpsHandler kicks off TLS request server
func tcpsHandler() {

	// Tell Notelib that we are serving TLS so it will reject TCP-only connect attempts
	notelib.RegisterTLSSupport()

	fmt.Printf("Serving requests on tcps:%s%s\n", serverAddress, serverPortTCPS)

	// Load our certs
	serviceCertFile, err := os.ReadFile(keyDirectory() + "hub.crt")
	if err != nil {
		fmt.Printf("tcps: error reading hub certificate: %s\n", err)
		return
	}
	serviceKeyFile, err := os.ReadFile(keyDirectory() + "hub.key")
	if err != nil {
		fmt.Printf("tcps: error reading hub key: %s\n", err)
		return
	}

	serviceKeyPair, err := tls.X509KeyPair(serviceCertFile, serviceKeyFile)
	if err != nil {
		fmt.Printf("tcps: error opening TLS service credentials: %s\n", err)
		return
	}

	// Cert pool for devices
	deviceCertPool := x509.NewCertPool()

	// Append STSAFE PRODUCTION cert
	deviceCA := notelib.GetNotecardSecureElementRootCertificateAsPEM()
	if err != nil {
		fmt.Printf("tcps: error opening cert for device verification: %s\n", err)
		return
	}
	deviceCertPool.AppendCertsFromPEM(deviceCA)

	// Build from cert pool
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serviceKeyPair},
		ClientCAs:    deviceCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	// Listen for incoming connect requests
	connServer, err := TLSListen("tcp", serverPortTCPS, tlsConfig)
	if err != nil {
		fmt.Printf("tcps: error listening on port %s: %s\n", serverPortTCPS, err)
		return
	}
	defer connServer.Close()

	for {

		// Accept the TCP connection
		connSession, err := connServer.Accept()
		if err != nil {
			fmt.Printf("\ntcps: error accepting TCPS session: %v\n", err)
			continue
		}

		// The scope of a TCP connection may be many requests, so dispatch
		// this to a goroutine which will deal with the session until it is closed.
		// Note that we also pass what port number we are on, so that handlers
		// are provisioned with the same port.
		go sessionHandler(connSession, true)

	}

}

// TLSListen creates a TLS listener accepting connections on the
// given network address using net.Listen.
// The configuration config must be non-nil and must include
// at least one certificate or else set GetCertificate.
func TLSListen(network, laddr string, config *tls.Config) (net.Listener, error) {
	if config == nil || (len(config.Certificates) == 0 && config.GetCertificate == nil) {
		return nil, errors.New("tls: neither Certificates nor GetCertificate set in Config")
	}
	l, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewTLSListener(l, config), nil
}

func printConnState(state tls.ConnectionState, message string) {
	fmt.Printf("TLS Connection Info for %s\n", message)
	fmt.Printf("					Version: %x\n", state.Version)
	fmt.Printf("		  HandshakeComplete: %t\n", state.HandshakeComplete)
	fmt.Printf("				  DidResume: %t\n", state.DidResume)
	fmt.Printf("				CipherSuite: %x\n", state.CipherSuite)
	fmt.Printf("		 NegotiatedProtocol: %s\n", state.NegotiatedProtocol)
	fmt.Printf("		  Certificate chain:\n")
	for i, cert := range state.PeerCertificates {
		subject := cert.Subject
		serial := cert.SerialNumber
		issuer := cert.Issuer
		fmt.Printf("				   %d s:%s\n", i, distinguishedName(subject, serial))
		fmt.Printf("					 i:%s\n", distinguishedName(issuer, nil))
	}
}

// Return a DN with only the components that we support
func distinguishedName(name pkix.Name, serial *big.Int) (dn string) {
	dn += name.String()
	if serial != nil {
		dn += fmt.Sprintf(" SERIALNUMBER=\"%s\"", serial.Text(16))
	}
	return
}

// A tlsListener implements a network listener (net.Listener) for TLS connections.
type tlsListener struct {
	net.Listener
	config *tls.Config
}

// Accept waits for and returns the next incoming TLS connection.
// The returned connection is of type *Conn.
func (l *tlsListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Set keepalive timers that cause TCP-level pings to be sent
	tcpc := c.(*net.TCPConn)
	tcpc.SetKeepAlive(true)
	tcpc.SetKeepAlivePeriod(15 * time.Minute)

	// Initiate the session
	conn := tls.Server(c, l.config)

	// Done
	return conn, nil
}

// NewTLSListener creates a Listener which accepts connections from an inner
// Listener and wraps each connection with Server.
// The configuration config must be non-nil and must include
// at least one certificate or else set GetCertificate.
func NewTLSListener(inner net.Listener, config *tls.Config) net.Listener {
	l := new(tlsListener)
	l.Listener = inner
	l.config = config
	return l
}

// Authenticate an incoming session from a device
func tlsAuthenticate(connSession net.Conn, device DeviceState) (err error) {

	// Get info about the session
	tlsc := connSession.(*tls.Conn)
	tlsst := tlsc.ConnectionState()

	// Validate that the negotiation succeeded
	if !tlsst.HandshakeComplete || len(tlsst.PeerCertificates) == 0 {
		printConnState(tlsc.ConnectionState(), device.DeviceUID)
		return fmt.Errorf("tls: connection not complete")
	}
	deviceDN := distinguishedName(tlsst.PeerCertificates[0].Subject, tlsst.PeerCertificates[0].SerialNumber)

	// Ensure that the device DN never changes once it has locked-on
	if device.DeviceDN != "" && deviceDN != device.DeviceDN {
		fmt.Printf("***** FAILED TLS AUTH for Device: %s\n", device.DeviceUID)
		fmt.Printf("*****			   Last Known DN: %s\n", device.DeviceDN)
		fmt.Printf("*****			TLS-Certified DN: %s\n", deviceDN)
		return fmt.Errorf("tls: cannot change (or spoof) DeviceUID of this device")
	}

	// If it's never been set, set it now
	if device.DeviceDN == "" {
		device.DeviceDN = deviceDN
		deviceSet(device)
		fmt.Printf("tcp: first-time auth of %s as %s\n", device.DeviceUID, deviceDN)
	} else {
		fmt.Printf("tcp: authenticated %s as %s\n", device.DeviceUID, deviceDN)
	}

	// Success
	return

}
