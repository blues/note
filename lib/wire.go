// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib wire.go handles all conversions between the RPC's req/rsp structure and compressed on-wire formats
package notelib

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
	"github.com/golang/snappy"
	olc "github.com/google/open-location-code/go"
	"golang.org/x/crypto/chacha20poly1305"
	"google.golang.org/protobuf/proto"
)

// Available JSON compression formats.
const (
	jc0       = byte(0) // no compression
	jc1       = byte(1) // replace the 5 json strings
	jc2       = byte(2) // replace the 5 json strings, then apply Snappy
	jc3       = byte(3) // json string subst table at top, replace the 5 or 6 json strings, then apply Snappy
	jcCurrent = jc3
)

// JSON compression strings
const (
	from3 = "\":{"
	from4 = "},\""
	from5 = "\":"
	from6 = ",\""
	from7 = "}}"
	from8 = "true"
	to1   = "\001"
	to2   = "\002"
	to3   = "\003"
	to4   = "\004"
	to5   = "\005"
	to6   = "\006"
	to7   = "\007"
	to8   = "\010"
)

// Debug
var debugWireRead = false

// Method to scan for the longest json field value within the buffer, for JSON field/value substitution,
// optimized for large ints (unix date values) or string with common prefixes
func scanForJSONValue(buf []byte, field string) (tokenBuf []byte) {
	scanFor := []byte("\"" + field + "\":")
	scanForLen := len(scanFor)
	occurrences := 0

	bufScanIndex := 0
	for bufScanIndex < len(buf) {

		// Look for the next occurrence of the field we're looking for
		i := bytes.Index(buf[bufScanIndex:], scanFor)
		if i == -1 {
			break
		}
		i += bufScanIndex

		// Look for the end of the data value
		token := []byte("")
		for j := i; j < len(buf); j++ {
			ch := buf[j]
			if ch == ' ' || ch == ',' || ch == '}' {
				break
			}
			token = append(token, ch)
		}

		// If we've not yet grabbed our first token, grab the entire thing
		if len(tokenBuf) == 0 {
			// For us to start looking for commonalities in a token,
			// it must be at least this many characters of savings.
			if (len(token) - scanForLen) >= 5 {
				tokenBuf = token
				occurrences = 1
			}
		} else {

			// Look for the longest token that we still have in common
			shrank := false
			for j := 0; j < len(tokenBuf); j++ {
				if j >= len(token) || tokenBuf[j] != token[j] {

					// To shrink the token length, the token must at least have SOME in common with the original.
					if (j - scanForLen) >= 4 {
						tokenBuf = tokenBuf[:j]
					}
					shrank = true
					break
				}
			}
			if !shrank {
				occurrences++
			}

		}

		// Update the pointer so that we look for the next one
		bufScanIndex = i + scanForLen

	}

	if debugCompress {
		logDebug(context.Background(), "#### longest token (%d) occurrences %d savings %d: %s",
			len(tokenBuf), occurrences, (occurrences*len(tokenBuf))-occurrences, tokenBuf)
	}

	return
}

// Compress a byte array known to contain JSON.  Note that we don't do any compression
// beyond 07 because old firmware doesn't support it.
func jsonCompress(normal []byte) (compressed []byte, err error) {
	// Begin generating output by using header byte specifying what kind of compression.
	// If it becomes advantageous to do so, this can be determined dynamically based upon
	// the compressability of the data using different algorithms.
	compressed = append([]byte{}, jcCurrent)

	// No compression
	if jcCurrent == jc0 {

		compressed = append(compressed, normal...)

		// Debug
		if debugCompress {
			logDebug(context.Background(), "JSON no compression (%d)", len(normal)+1)
		}

		return

	}

	// Generate the compression length table for jc3
	from1 := []byte("")
	from2 := []byte("")
	substTable := []byte{}
	if jcCurrent == jc3 {
		from1 = scanForJSONValue(normal, "w")
		from2 = scanForJSONValue(normal, "l")
		substTable = make([]byte, 1+(2*2)) // Number of "from/to" entries, plus fromlen/tolen for each
		substTable[0] = 2
		substTable[1] = byte(len(from1))
		substTable[2] = byte(len(to1))
		substTable[3] = byte(len(from2))
		substTable[4] = byte(len(to2))
		substTable = append(substTable, from1...)
		substTable = append(substTable, to1...)
		substTable = append(substTable, from2...)
		substTable = append(substTable, to2...)
	}

	// Do the substitutions
	str := string(normal)
	if jcCurrent == jc3 {
		if len(from1) > 0 {
			str = strings.Replace(str, string(from1), to1, -1)
		}
		if len(from2) > 0 {
			str = strings.Replace(str, string(from2), to2, -1)
		}
	}
	str = strings.Replace(str, from3, to3, -1)
	str = strings.Replace(str, from4, to4, -1)
	str = strings.Replace(str, from5, to5, -1)
	str = strings.Replace(str, from6, to6, -1)
	str = strings.Replace(str, from7, to7, -1)
	jcompressed := []byte(str)

	// Now that the other replacements have been done, insert the subst table at the front
	if jcCurrent == jc3 {
		jcompressed = append(substTable, jcompressed...)
	}

	// JSON compression followed by Snappy compression
	if debugCompress {
		logDebug(context.Background(), "  JSON compressed from %d to %d", len(normal), len(jcompressed))
	}

	// Snappy
	if jcCurrent == jc1 {

		compressed = append(compressed, jcompressed...)

		if debugCompress {
			logDebug(context.Background(), " plus header byte from %d to %d", len(jcompressed), len(compressed))
		}

	} else {

		scompressed := snappy.Encode(nil, jcompressed)
		compressed = append(compressed, scompressed...)

		if debugCompress {
			logDebug(context.Background(), "      plus Snappy from %d to %d", len(jcompressed), len(scompressed))
			logDebug(context.Background(), " plus header byte from %d to %d", len(scompressed), len(compressed))
		}

	}

	return
}

// Decompress a byte array known to contain JSON.  Note that we support 08 as of 11/01/2020.
func jsonDecompress(compressed []byte) (normal []byte, err error) {
	// Remove header byte
	if len(compressed) == 0 {
		return nil, fmt.Errorf("json decompression error: 0-length data")
	}

	compressionType := compressed[0]
	compressed = compressed[1:]

	// Dispatch based on compression type
	if compressionType == jc0 {
		normal = compressed

		// Debug
		if debugCompress {
			logDebug(context.Background(), "No decompression (%d)", len(normal))
		}
		return
	}

	if debugCompress {
		logDebug(context.Background(), " Removed header byte from %d to %d", len(compressed)+1, len(compressed))
	}

	sdecompressed := compressed
	if compressionType == jc1 {
		// only json
	} else if compressionType == jc2 || compressionType == jc3 {

		// Snappy decompress
		sdecompressed, err = snappy.Decode(nil, compressed)
		if err != nil {
			return nil, fmt.Errorf("json decompression decode error: %s", err)
		}

		if debugCompress {
			logDebug(context.Background(), " Snappy decompressed from %d to %d", len(compressed), len(sdecompressed))
		}

	} else {
		return nil, fmt.Errorf("json decompression error: unknown compression type: 0x%02x", compressionType)
	}

	// JSON decompress

	var jdecompressed []byte
	if compressionType == jc3 {

		// Compute the total length of the subst table, and copy all strings to an array
		fromto := []string{}
		substTableEntries := sdecompressed[0]
		substTableLen := 1 + (substTableEntries * 2)
		substLenTableOffset := 1
		substStringsOffset := substTableLen
		for i := 0; i < int(substTableEntries*2); i++ {
			stringLen := sdecompressed[substLenTableOffset+i]
			substStr := string(sdecompressed[substStringsOffset:(substStringsOffset + stringLen)])
			fromto = append(fromto, substStr)
			substStringsOffset += stringLen
			substTableLen += stringLen
		}

		// Eliminate the subst table from the output buffer
		sdecompressed = sdecompressed[substTableLen:]
		str := string(sdecompressed)

		// Perform the hard-wired substitutions
		str = strings.Replace(str, to8, from8, -1)
		str = strings.Replace(str, to7, from7, -1)
		str = strings.Replace(str, to6, from6, -1)
		str = strings.Replace(str, to5, from5, -1)
		str = strings.Replace(str, to4, from4, -1)
		str = strings.Replace(str, to3, from3, -1)

		// Perform the reverse substitution from "to" to "from", in reverse order
		for i := int(substTableEntries); i > 0; i-- {
			from := fromto[((i-1)*2)+0]
			to := fromto[((i-1)*2)+1]
			if len(from) > 0 {
				str = strings.Replace(str, to, from, -1)
			}
		}
		jdecompressed = []byte(str)

	} else {

		str := string(sdecompressed)
		str = strings.Replace(str, to8, from8, -1)
		str = strings.Replace(str, to7, from7, -1)
		str = strings.Replace(str, to6, from6, -1)
		str = strings.Replace(str, to5, from5, -1)
		str = strings.Replace(str, to4, from4, -1)
		str = strings.Replace(str, to3, from3, -1)
		jdecompressed = []byte(str)

	}

	// Debug
	if debugCompress {
		logDebug(context.Background(), "           then JSON from %d to %d", len(sdecompressed), len(jdecompressed))
	}

	normal = jdecompressed
	return
}

// SetNotefile sets the notefile within the a notehubMessage data structure
func (msg *notehubMessage) SetNotefile(notefile *Notefile) error {
	// Set compressed form
	JSON, err := note.JSONMarshal(notefile)
	if err != nil {
		return err
	}
	if msg.cf == nil {
		msg.cf = &cf{}
	}
	msg.cf.Notefile, err = jsonCompress(JSON)

	return err
}

// GetNotefile gets the notefile from within the a notehubMessage data structure
func (msg *notehubMessage) GetNotefile() (notefile *Notefile, err error) {
	notefile = &Notefile{}

	// If native is available, return it, else decompress
	if msg.nf != nil && msg.nf.Notefile != nil {
		notefile = msg.nf.Notefile
	} else {
		if msg.cf == nil || len(msg.cf.Notefile) == 0 {
			return
		}
		jdata, err2 := jsonDecompress(msg.cf.Notefile)
		if err2 != nil {
			err = err2
			return
		}
		err = note.JSONUnmarshal(jdata, notefile)
		if err != nil {
			return
		}
	}

	// If there's an externalized payload, internalize it
	if msg.nf.Payload != nil && len(*msg.nf.Payload) != 0 {
		err = notefile.InternalizePayload(*msg.nf.Payload)
		if err != nil {
			return
		}
	}

	// Done
	return
}

// SetNotefiles sets the notefile list within the a notehubMessage data structure
func (msg *notehubMessage) SetNotefiles(notefiles map[string]*Notefile) error {
	// Set native form
	if msg.nf == nil {
		msg.nf = &nf{}
	}
	msg.nf.Notefiles = &notefiles

	// Set compressed form
	JSON, err := note.JSONMarshal(notefiles)
	if err != nil {
		return err
	}
	if msg.cf == nil {
		msg.cf = &cf{}
	}
	msg.cf.Notefiles, err = jsonCompress(JSON)

	return err
}

// GetNotefiles gets the multi-notefile structure from within the a notehubMessage
func (msg *notehubMessage) GetNotefiles() (notefiles map[string]*Notefile, err error) {
	notefiles = map[string]*Notefile{}

	// If native is available, return it, else decompress
	if msg.nf != nil && msg.nf.Notefiles != nil {
		notefiles = *msg.nf.Notefiles
	} else {
		if msg.cf == nil || len(msg.cf.Notefiles) == 0 {
			return
		}
		jdata, err2 := jsonDecompress(msg.cf.Notefiles)
		if err2 != nil {
			err = err2
			return
		}
		err = note.JSONUnmarshal(jdata, notefiles)
		if err != nil {
			return
		}
	}

	// If there's an externalized payload, internalize it
	if msg.nf.Payload != nil && len(*msg.nf.Payload) != 0 {
		for _, notefile := range notefiles {
			err = notefile.InternalizePayload(*msg.nf.Payload)
			if err != nil {
				return
			}
		}
	}

	// Done
	return
}

// SetBody sets the body within the a notehubMessage data structure
func (msg *notehubMessage) SetBody(body map[string]interface{}) error {
	// Set native form
	if msg.nf == nil {
		msg.nf = &nf{}
	}
	msg.nf.Body = &body

	// Set compressed form
	JSON, err := note.JSONMarshal(body)
	if err != nil {
		return err
	}
	if msg.cf == nil {
		msg.cf = &cf{}
	}
	msg.cf.Body, err = jsonCompress(JSON)

	return err
}

// GetBody gets the body from within the a notehubMessage
func (msg *notehubMessage) GetBody() (body map[string]interface{}, err error) {
	body = map[string]interface{}{}

	// If native is available, return it
	if msg.nf != nil && msg.nf.Body != nil {
		body = *msg.nf.Body
		return
	}

	// If compressed is available, return it
	if msg.cf != nil && len(msg.cf.Body) != 0 {
		jdata, err2 := jsonDecompress(msg.cf.Body)
		if err2 != nil {
			err = err2
		} else {
			err = note.JSONUnmarshal(jdata, &body)
		}
		return
	}

	return
}

// SetPayload sets the payload within the a notehubMessage data structure
func (msg *notehubMessage) SetPayload(payload []byte) {
	// Set native form
	if msg.nf == nil {
		msg.nf = &nf{}
	}
	msg.nf.Payload = &payload

	// Set compressed form
	if msg.cf == nil {
		msg.cf = &cf{}
	}
	msg.cf.Payload = payload
}

// GetPayload gets the payload from within the a notehubMessage
func (msg *notehubMessage) GetPayload() (payload []byte) {
	payload = []byte{}

	// If native is available, return it
	if msg.nf != nil && msg.nf.Payload != nil {
		payload = *msg.nf.Payload
	}

	// If compressed is available, use that
	if msg.cf != nil && len(msg.cf.Payload) != 0 {
		payload = msg.cf.Payload
		return
	}

	return
}

// wireReadVersionByte reads the initial byte of a stream to validate version
func wireProcessVersionByte(version byte) (isValid bool, headerLength int) {
	switch version {

	// 1 byte version == 0
	// 0 byte protobuf length
	// 0 byte binary length
	// 0 bytes protobuf
	// 0 bytes binary
	case 0:
		return true, 0

		// 1 byte version == 1
		// 1 byte protobuf length
		// 1 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 1:
		return true, 2

		// 1 byte version == 2
		// 1 byte protobuf length
		// 2 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 2:
		return true, 3

		// 1 byte version == 3
		// 2 byte protobuf length
		// 2 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 3:
		return true, 4

		// 1 byte version == 4
		// 4 byte protobuf length
		// 4 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 4:
		return true, 8

		// 1 byte version == 5
		// 4 byte binary length
		// N bytes binary
	case 5:
		return true, 4

	}

	return false, 0
}

// wireMake creates a header from the specified parameters
func wireMake(protobuf *[]byte, binary *[]byte) (out []byte) {
	protobufLength := len(*protobuf)
	binaryLength := len(*binary)

	// Make the header
	var header []byte
	if protobufLength == 0 && binaryLength == 0 {
		header = make([]byte, 1)
		header[0] = 0
	} else if protobufLength == 0 && binaryLength != 0 {
		header = make([]byte, 5)
		header[0] = 5
		header[1] = byte(binaryLength & 0x0ff)
		header[2] = byte((binaryLength >> 8) & 0x0ff)
		header[3] = byte((binaryLength >> 16) & 0x0ff)
		header[4] = byte((binaryLength >> 24) & 0x0ff)
	} else if protobufLength < 256 && binaryLength < 256 {
		header = make([]byte, 3)
		header[0] = 1
		header[1] = byte(protobufLength)
		header[2] = byte(binaryLength)
	} else if protobufLength < 256 && binaryLength < 65536 {
		header = make([]byte, 4)
		header[0] = 2
		header[1] = byte(protobufLength)
		header[2] = byte(binaryLength & 0x0ff)
		header[3] = byte((binaryLength >> 8) & 0x0ff)
	} else if protobufLength < 65536 && binaryLength < 65536 {
		header = make([]byte, 5)
		header[0] = 3
		header[1] = byte(protobufLength & 0x0ff)
		header[2] = byte((protobufLength >> 8) & 0x0ff)
		header[3] = byte(binaryLength & 0x0ff)
		header[4] = byte((binaryLength >> 8) & 0x0ff)
	} else {
		header = make([]byte, 9)
		header[0] = 4
		header[1] = byte(protobufLength & 0x0ff)
		header[2] = byte((protobufLength >> 8) & 0x0ff)
		header[3] = byte((protobufLength >> 16) & 0x0ff)
		header[4] = byte((protobufLength >> 24) & 0x0ff)
		header[5] = byte(binaryLength & 0x0ff)
		header[6] = byte((binaryLength >> 8) & 0x0ff)
		header[7] = byte((binaryLength >> 16) & 0x0ff)
		header[8] = byte((binaryLength >> 24) & 0x0ff)
	}

	// Make the aggregate object
	headerLength := len(header)
	out = make([]byte, headerLength+protobufLength+binaryLength)
	copy(out[0:], header)
	copy(out[headerLength:], *protobuf)
	copy(out[headerLength+protobufLength:], *binary)

	// Done
	return
}

// msgToWire converts a request to wire format
func msgToWire(msg notehubMessage) (wire []byte, wirelen int, err error) {
	// Create the PB header
	pb := NotehubPB{}
	if msg.Version != 0 {
		version := int64(msg.Version)
		pb.Version = &version
	}
	if msg.MessageType != "" {
		pb.MessageType = &msg.MessageType
	}
	if msg.Error != "" {
		pb.Error = &msg.Error
	}
	if msg.DeviceUID != "" {
		pb.DeviceUID = &msg.DeviceUID
	}
	if msg.DeviceSN != "" {
		pb.DeviceSN = &msg.DeviceSN
	}
	if msg.DeviceSKU != "" {
		pb.DeviceSKU = &msg.DeviceSKU
	}
	if msg.DeviceOrderingCode != "" {
		pb.DeviceOrderingCode = &msg.DeviceOrderingCode
	}
	if msg.DeviceFirmware != 0 {
		pb.DeviceFirmware = &msg.DeviceFirmware
	}
	if msg.DevicePIN != "" {
		pb.DevicePIN = &msg.DevicePIN
	}
	if msg.ProductUID != "" {
		pb.ProductUID = &msg.ProductUID
	}
	if msg.DeviceEndpointID != "" {
		pb.DeviceEndpointID = &msg.DeviceEndpointID
	}
	if msg.HubTimeNs != 0 {
		pb.HubTimeNs = &msg.HubTimeNs
	}
	if msg.HubEndpointID != "" {
		pb.HubEndpointID = &msg.HubEndpointID
	}
	if msg.Where != "" {
		pb.Where = &msg.Where
	}
	if msg.WhereWhen != 0 {
		pb.WhereWhen = &msg.WhereWhen
	}
	if msg.HubPacketHandler != "" {
		pb.HubPacketHandler = &msg.HubPacketHandler
	}
	if msg.HubSessionHandler != "" {
		pb.HubSessionHandler = &msg.HubSessionHandler
	}
	if msg.HubSessionFactoryResetID != "" {
		pb.HubSessionFactoryResetID = &msg.HubSessionFactoryResetID
	}
	if msg.HubSessionTicket != "" {
		pb.HubSessionTicket = &msg.HubSessionTicket
	}
	if msg.HubSessionTicketExpiresTimeSec != 0 {
		pb.HubSessionTicketExpiresTimeSec = &msg.HubSessionTicketExpiresTimeSec
	}
	if msg.NotefileID != "" {
		pb.NotefileID = &msg.NotefileID
	}
	if msg.NotefileIDs != "" {
		pb.NotefileIDs = &msg.NotefileIDs
	}
	if msg.Since != 0 {
		pb.Since = &msg.Since
	}
	if msg.Until != 0 {
		pb.Until = &msg.Until
	}
	if msg.MaxChanges != 0 {
		maxchanges := int64(msg.MaxChanges)
		pb.MaxChanges = &maxchanges
	}
	if msg.NoteID != "" {
		pb.NoteID = &msg.NoteID
	}
	if msg.SessionIDPrev != 0 {
		pb.SessionIDPrev = &msg.SessionIDPrev
	}
	if msg.SessionIDNext != 0 {
		pb.SessionIDNext = &msg.SessionIDNext
	}
	if msg.SessionIDMismatch {
		pb.SessionIDMismatch = &msg.SessionIDMismatch
	}
	if msg.NotificationSession {
		pb.NotificationSession = &msg.NotificationSession
	}
	if msg.ContinuousSession {
		pb.ContinuousSession = &msg.ContinuousSession
	}
	if msg.SuppressResponse {
		pb.SuppressResponse = &msg.SuppressResponse
	}
	if msg.Voltage100 != 0 {
		pb.Voltage100 = &msg.Voltage100
	}
	if msg.Temp100 != 0 {
		pb.Temp100 = &msg.Temp100
	}
	if msg.Voltage1000 != 0 {
		pb.Voltage1000 = &msg.Voltage1000
	}
	if msg.Temp1000 != 0 {
		pb.Temp1000 = &msg.Temp1000
	}
	if msg.CellID != "" {
		pb.CellID = &msg.CellID
	}
	if msg.UsageProvisioned != 0 {
		pb.UsageProvisioned = &msg.UsageProvisioned
	}
	if msg.UsageRcvdBytes != 0 {
		pb.UsageRcvdBytes = &msg.UsageRcvdBytes
	}
	if msg.UsageSentBytes != 0 {
		pb.UsageSentBytes = &msg.UsageSentBytes
	}
	if msg.UsageRcvdBytesSecondary != 0 {
		pb.UsageRcvdBytesSecondary = &msg.UsageRcvdBytesSecondary
	}
	if msg.UsageSentBytesSecondary != 0 {
		pb.UsageSentBytesSecondary = &msg.UsageSentBytesSecondary
	}
	if msg.UsageTCPSessions != 0 {
		pb.UsageTCPSessions = &msg.UsageTCPSessions
	}
	if msg.UsageTLSSessions != 0 {
		pb.UsageTLSSessions = &msg.UsageTLSSessions
	}
	if msg.UsageRcvdNotes != 0 {
		pb.UsageRcvdNotes = &msg.UsageRcvdNotes
	}
	if msg.UsageSentNotes != 0 {
		pb.UsageSentNotes = &msg.UsageSentNotes
	}
	if msg.HighPowerSecsTotal != 0 {
		pb.HighPowerSecsTotal = &msg.HighPowerSecsTotal
	}
	if msg.HighPowerSecsData != 0 {
		pb.HighPowerSecsData = &msg.HighPowerSecsData
	}
	if msg.HighPowerSecsGPS != 0 {
		pb.HighPowerSecsGPS = &msg.HighPowerSecsGPS
	}
	if msg.HighPowerCyclesTotal != 0 {
		pb.HighPowerCyclesTotal = &msg.HighPowerCyclesTotal
	}
	if msg.HighPowerCyclesData != 0 {
		pb.HighPowerCyclesData = &msg.HighPowerCyclesData
	}
	if msg.HighPowerCyclesGPS != 0 {
		pb.HighPowerCyclesGPS = &msg.HighPowerCyclesGPS
	}
	if msg.MotionSecs != 0 {
		pb.MotionSecs = &msg.MotionSecs
	}
	if msg.MotionOrientation != "" {
		pb.MotionOrientation = &msg.MotionOrientation
	}
	if msg.SessionTrigger != "" {
		pb.SessionTrigger = &msg.SessionTrigger
	}

	// Create the binary object
	if msg.cf == nil {
		msg.cf = &cf{}
	}
	lenBytes1Notefile := int64(0)
	if msg.cf.Notefile != nil {
		lenBytes1Notefile = int64(len(msg.cf.Notefile))
		if lenBytes1Notefile != 0 {
			pb.Bytes1 = &lenBytes1Notefile
		}
	}
	lenBytes2Notefiles := int64(0)
	if msg.cf.Notefiles != nil {
		lenBytes2Notefiles = int64(len(msg.cf.Notefiles))
		if lenBytes2Notefiles != 0 {
			pb.Bytes2 = &lenBytes2Notefiles
		}
	}
	lenBytes3Body := int64(0)
	if msg.cf.Body != nil {
		lenBytes3Body = int64(len(msg.cf.Body))
		if lenBytes3Body != 0 {
			pb.Bytes3 = &lenBytes3Body
		}
	}
	lenBytes4Payload := int64(0)
	if msg.cf.Payload != nil {
		lenBytes4Payload = int64(len(msg.cf.Payload))
		if lenBytes4Payload != 0 {
			pb.Bytes4 = &lenBytes4Payload
		}
	}
	binaryLength := lenBytes1Notefile + lenBytes2Notefiles + lenBytes3Body + lenBytes4Payload
	bindata := make([]byte, binaryLength)
	if msg.cf.Notefile != nil {
		copy(bindata[0:], msg.cf.Notefile)
	}
	if msg.cf.Notefiles != nil {
		copy(bindata[lenBytes1Notefile:], msg.cf.Notefiles)
	}
	if msg.cf.Body != nil {
		copy(bindata[lenBytes1Notefile+lenBytes2Notefiles:], msg.cf.Body)
	}
	if msg.cf.Payload != nil {
		copy(bindata[lenBytes1Notefile+lenBytes2Notefiles+lenBytes3Body:], msg.cf.Payload)
	}

	// Generate the PB
	pbdata, pberr := proto.Marshal(&pb)
	if pberr != nil {
		err = pberr
		return
	}

	// Generate the wire object from those two buffers
	wire = wireMake(&pbdata, &bindata)
	wirelen = len(wire)

	return
}

// wireProcessHeader extracts protocol buffer and binary lengths from the header
func wireReadHeader(version byte, header []byte) (protobufLength int64, binaryLength int64, err error) {
	var isValidVersion bool

	switch version {

	// 1 byte version == 0
	// 0 byte protobuf length
	// 0 byte binary length
	// 0 bytes protobuf
	// 0 bytes binary
	case 0:
		protobufLength = 0
		binaryLength = 0
		isValidVersion = true

		// 1 byte version == 1
		// 1 byte protobuf length
		// 1 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 1:
		protobufLength = int64(header[0])
		binaryLength = int64(header[1])
		isValidVersion = true

		// 1 byte version == 2
		// 1 byte protobuf length
		// 2 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 2:
		protobufLength = int64(header[0])
		binaryLength = (int64(header[2]) << 8) | int64(header[1])
		isValidVersion = true

		// 1 byte version == 3
		// 2 byte protobuf length
		// 2 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 3:
		protobufLength = (int64(header[1]) << 8) | int64(header[0])
		binaryLength = (int64(header[3]) << 8) | int64(header[2])
		isValidVersion = true

		// 1 byte version == 4
		// 4 byte protobuf length
		// 4 byte binary length
		// N bytes protobuf
		// N bytes binary
	case 4:
		protobufLength = (int64(header[3]) << 24) | (int64(header[2]) << 16) | (int64(header[1]) << 8) | int64(header[0])
		binaryLength = (int64(header[7]) << 24) | (int64(header[6]) << 16) | (int64(header[5]) << 8) | int64(header[4])
		isValidVersion = true

		// 1 byte version == 5
		// 4 byte binary length
		// N bytes binary
	case 5:
		protobufLength = 0
		binaryLength = (int64(header[3]) << 24) | (int64(header[2]) << 16) | (int64(header[1]) << 8) | int64(header[0])
		isValidVersion = true
	}

	if !isValidVersion {
		err = fmt.Errorf("wire: Invalid header version %d", version)
		return
	}

	// Validate the header length fields, knowing that this is an RPC coming from
	// the memory of a microcontroller with far less than 1MB of SRAM.  Note that
	// this doesn't guarantee that the header is valid, but it definitely will
	// return false if it's obviously invalid.  A fuzzer will definitely still
	// be able to send us garbage.

	// Our protobufs are quite small and have no variable-length data
	if protobufLength < 0 || protobufLength > 10000 {
		err = fmt.Errorf("wire: protobufLength out of range %d", protobufLength)
		return
	}

	// Our binary generally contains a Notefile structure, and can't
	// possibly be larger than the memory of the microcontroller.  Since
	// as of 2022 we currently restrict our firmware to use 512KB of SRAM,
	// a 2MB check should be quite reasonable.
	if binaryLength < 0 || binaryLength > 2*1024*1024 {
		err = fmt.Errorf("wire: binaryLength out of range %d", protobufLength)
		return
	}

	return
}

// u32min returns the smaller of x or y.
func u32min(x, y uint32) uint32 {
	if x > y {
		return y
	}
	return x
}

// WireBarsFromSession extracts device's perception of the number of bars of signal from a session
func WireBarsFromSession(session *HubSession) (rat string, bars uint32) {
	// Return the rat for the session
	rat = session.Session.Rat

	// Start by assuming great coverage
	bars = 4

	// Handle GSM OR handle LTE at the state when RSRQ can't be computed
	if session.Session.Rsrq == 0 {
		if session.Session.Rssi < -70 {
			bars = 3
		}
		if session.Session.Rssi < -85 {
			bars = 2
		}
		if session.Session.Rssi < -100 {
			bars = 1
		}
		return
	}

	// RSRP is an integer indicating the reference signal received power in dBm
	if session.Session.Rsrp < -80 {
		bars = u32min(bars, 3)
	}
	if session.Session.Rsrp < -90 {
		bars = u32min(bars, 2)
	}
	if session.Session.Rsrp < -100 {
		bars = u32min(bars, 1)
	}
	// SINR is an integer indicating the signal to interference plus noise ratio.
	// The logarithmic values (0-250) are in 1/5th of a dB, ranging from -20 to +30db
	sinr := -20 + (session.Session.Sinr * 5)
	if sinr < 20 {
		bars = u32min(bars, 3)
	}
	if sinr < 13 {
		bars = u32min(bars, 2)
	}
	if sinr <= 0 {
		bars = u32min(bars, 1)
	}
	// RSRQ is an integer indicating the reference signal received quality (RSRQ) in dB,
	// which is computed by the formula RSRQ = N*(RSRP/RSSI), where N is the number of
	// Resource Blocks of the E-UTRA carrier RSSI
	if session.Session.Rsrq < -10 {
		bars = u32min(bars, 3)
	}
	if session.Session.Rsrq < -15 {
		bars = u32min(bars, 2)
	}
	if session.Session.Rsrq < -20 {
		bars = u32min(bars, 1)
	}

	// Done
	return
}

// WireExtractSessionContext extracts session context from the wire message
func WireExtractSessionContext(wire []byte, session *HubSession) (suppressResponse bool, err error) {
	var req notehubMessage
	req, _, err = msgFromWire(wire)
	if err != nil {
		return
	}

	// If the productUID is already set for this session leave it alone.  This
	// covers the case in which the notehub implementation wishes to redirect
	// one productUID to another.
	if session.Session.ProductUID == "" {
		session.Session.ProductUID = req.ProductUID
	}

	// Extract the remainder of the session context
	suppressResponse = req.SuppressResponse
	session.Session.DeviceUID = req.DeviceUID
	session.Session.DeviceSN = req.DeviceSN
	session.DeviceSKU = req.DeviceSKU
	session.DeviceOrderingCode = req.DeviceOrderingCode
	session.DeviceFirmware = req.DeviceFirmware
	session.DevicePIN = req.DevicePIN
	session.DeviceEndpointID = req.DeviceEndpointID
	session.HubEndpointID = req.HubEndpointID
	session.HubSessionTicket = req.HubSessionTicket
	session.FactoryResetID = req.HubSessionFactoryResetID
	session.Session.This().Since = req.UsageProvisioned
	session.Session.This().RcvdBytes = req.UsageRcvdBytes
	session.Session.This().SentBytes = req.UsageSentBytes
	session.Session.This().RcvdBytesSecondary = req.UsageRcvdBytesSecondary
	session.Session.This().SentBytesSecondary = req.UsageSentBytesSecondary
	session.Session.This().TCPSessions = req.UsageTCPSessions
	session.Session.This().TLSSessions = req.UsageTLSSessions
	session.Session.This().RcvdNotes = req.UsageRcvdNotes
	session.Session.This().SentNotes = req.UsageSentNotes
	session.Session.HighPowerSecsTotal = req.HighPowerSecsTotal
	session.Session.HighPowerSecsData = req.HighPowerSecsData
	session.Session.HighPowerSecsGPS = req.HighPowerSecsGPS
	session.Session.HighPowerCyclesTotal = req.HighPowerCyclesTotal
	session.Session.HighPowerCyclesData = req.HighPowerCyclesData
	session.Session.HighPowerCyclesGPS = req.HighPowerCyclesGPS
	session.Session.Voltage = float64(req.Voltage100) / 100
	session.Session.Temp = float64(req.Temp100) / 100
	if req.Voltage1000 != 0 {
		session.Session.Voltage = float64(req.Voltage1000) / 1000
	}
	if req.Temp1000 != 0 {
		session.Session.Temp = float64(req.Temp1000) / 1000
	}
	session.Session.Moved = req.MotionSecs
	session.Session.Orientation = req.MotionOrientation
	session.Session.Trigger = req.SessionTrigger
	session.Notification = req.NotificationSession
	session.Session.ContinuousSession = req.ContinuousSession
	if req.MessageType == msgDiscover {
		session.Discovery = true
	}
	if req.Where != "" {
		session.Session.WhereOLC = req.Where
		session.Session.WhereWhen = req.WhereWhen
		area, err := olc.Decode(session.Session.WhereOLC)
		if err == nil {
			session.Session.WhereLat, session.Session.WhereLon = area.Center()
		}
	}

	// This complicated sequence is to perform this Sscanf, except that golang's Sscanf is so aggressive
	// that the first %s eats the remainder of the string and does not pay attention to the "," after
	// it inside the format string. Apparently, %s only works at the end of a format string.
	// fmt.Sscanf(req.CellID, "%d,%d,%d,%d,%d,%d,%d,%d,%s,%d,%d,%s,%s,%s",
	//     &mcc, &mnc, &lac, &cellid, &rssi, &sinr, &rsrp, &rsrq, &rat, &bearer, &bars, &str1, &str2, &ip)
	var mcc, mnc, lac, cellid, rssi, sinr, rsrp, rsrq, bearer, bars int
	var rat, str1, str2, ip string
	s := strings.Split(req.CellID, ",")
	if len(s) > 0 {
		mcc, _ = strconv.Atoi(s[0])
	}
	if len(s) > 1 {
		mnc, _ = strconv.Atoi(s[1])
	}
	if len(s) > 2 {
		lac, _ = strconv.Atoi(s[2])
	}
	if len(s) > 3 {
		cellid, _ = strconv.Atoi(s[3])
	}
	if len(s) > 4 {
		rssi, _ = strconv.Atoi(s[4])
	}
	if len(s) > 5 {
		sinr, _ = strconv.Atoi(s[5])
	}
	if len(s) > 6 {
		rsrp, _ = strconv.Atoi(s[6])
	}
	if len(s) > 7 {
		rsrq, _ = strconv.Atoi(s[7])
	}
	if len(s) > 8 {
		rat = s[8]
	}
	if len(s) > 9 {
		bearer, _ = strconv.Atoi(s[9])
	}
	if len(s) > 10 {
		bars, _ = strconv.Atoi(s[10])
	}
	if len(s) > 11 {
		str1 = s[11]
	}
	if len(s) > 12 {
		str2 = s[12]
	}
	if len(s) > 13 {
		ip = s[13]
	}

	// Set the network information parsed by CellID
	if req.CellID == "" || mcc == 0 {
		session.Session.CellID = ""
	} else {
		session.Session.CellID = fmt.Sprintf("%d,%d,%d,%d", mcc, mnc, lac, cellid)
	}
	session.Session.Rssi = rssi
	session.Session.Sinr = sinr
	session.Session.Rsrp = rsrp
	session.Session.Rsrq = rsrq
	session.Session.Rat = rat
	session.Session.Bars = bars
	session.Session.Ip = ip
	if bearer == notecard.NetworkBearerWLan {
		session.Session.Bssid = str1
		session.Session.Ssid = str2
	} else {
		session.Session.Iccid = str1
		session.Session.Apn = str2
	}
	switch bearer {
	case notecard.NetworkBearerGsm:
		session.Session.Bearer = "GSM"
	case notecard.NetworkBearerTdScdma:
		session.Session.Bearer = "TD-SCDMA"
	case notecard.NetworkBearerWcdma:
		session.Session.Bearer = "WCDMA"
	case notecard.NetworkBearerCdma2000:
		session.Session.Bearer = "CDMA2000"
	case notecard.NetworkBearerWiMax:
		session.Session.Bearer = "WIMAX"
	case notecard.NetworkBearerLteTdd:
		session.Session.Bearer = "LTE TDD"
	case notecard.NetworkBearerLteFdd:
		session.Session.Bearer = "LTE FDD"
	case notecard.NetworkBearerNBIot:
		session.Session.Bearer = "NB-IoT"
	case notecard.NetworkBearerWLan:
		session.Session.Bearer = "WiFi"
	case notecard.NetworkBearerBluetooth:
		session.Session.Bearer = "Bluetooth"
	case notecard.NetworkBearerIeee802p15p4:
		session.Session.Bearer = "IEEE 802.15.4"
	case notecard.NetworkBearerEthernet:
		session.Session.Bearer = "Ethernet"
	case notecard.NetworkBearerDsl:
		session.Session.Bearer = "DSL"
	case notecard.NetworkBearerPlc:
		session.Session.Bearer = "PLC"
	case notecard.NetworkBearerUnknown:
		session.Session.Bearer = "unknown"
	}

	// Devices prior to 13701 did this on ALL transactions, but it really
	// should *only* be performed on NoteboxSummary and Ping transactions
	// so that we don't interpret arbitrary payloads (on restarted-sessions)
	// as scan results.  Builds after 13701 only send this on pings.
	if req.nf.Payload != nil && (req.MessageType == msgPing || req.MessageType == msgNoteboxSummary) {
		session.Session.ScanResults = req.nf.Payload
	}

	return
}

// msgFromWire converts a request from wire format
func msgFromWire(wire []byte) (msg notehubMessage, wirelen int, err error) {
	msg = notehubMessage{}

	// Process the header
	wirebuflen := len(wire)
	if wirebuflen < 1 {
		err = fmt.Errorf("wire: can't read version")
		return
	}
	wirever := wire[0]
	isValid, hdrlen := wireProcessVersionByte(wirever)
	if !isValid {
		err = fmt.Errorf("wire: invalid version")
		return
	}
	if wirebuflen < (1 + hdrlen) {
		err = fmt.Errorf("wire: can't read header")
		return
	}
	protobufLength, binaryLength, err := wireReadHeader(wirever, wire[1:1+hdrlen])
	if err != nil {
		return
	}

	// Verify that there's enough to read all the data
	if wirebuflen < (1 + hdrlen + int(protobufLength+binaryLength)) {
		err = fmt.Errorf("wire: message is too short")
		return
	}

	// Parse the protocol buffer
	pb := NotehubPB{}
	wirebase := 1 + hdrlen
	err = proto.Unmarshal(wire[wirebase:wirebase+int(protobufLength)], &pb)
	if err != nil {
		err = fmt.Errorf("wire: cannot unmarshal PB: %s", err)
		return
	}
	wirebase += int(protobufLength)

	// Extract the PB
	msg.Version = uint32(pb.GetVersion())
	msg.MessageType = pb.GetMessageType()
	msg.Error = pb.GetError()
	msg.DeviceUID = pb.GetDeviceUID()
	msg.DeviceSN = pb.GetDeviceSN()
	msg.DeviceSKU = pb.GetDeviceSKU()
	msg.DeviceOrderingCode = pb.GetDeviceOrderingCode()
	msg.DeviceFirmware = pb.GetDeviceFirmware()
	msg.DevicePIN = pb.GetDevicePIN()
	msg.ProductUID = pb.GetProductUID()
	msg.DeviceEndpointID = pb.GetDeviceEndpointID()
	msg.HubTimeNs = pb.GetHubTimeNs()
	msg.HubEndpointID = pb.GetHubEndpointID()
	msg.HubPacketHandler = pb.GetHubPacketHandler()
	msg.HubSessionHandler = pb.GetHubSessionHandler()
	msg.HubSessionFactoryResetID = pb.GetHubSessionFactoryResetID()
	msg.HubSessionTicket = pb.GetHubSessionTicket()
	msg.HubSessionTicketExpiresTimeSec = pb.GetHubSessionTicketExpiresTimeSec()
	msg.Where = pb.GetWhere()
	msg.WhereWhen = pb.GetWhereWhen()
	msg.NotefileID = pb.GetNotefileID()
	msg.NotefileIDs = pb.GetNotefileIDs()
	msg.Since = pb.GetSince()
	msg.Until = pb.GetUntil()
	msg.MaxChanges = int32(pb.GetMaxChanges())
	msg.NoteID = pb.GetNoteID()
	msg.SessionIDPrev = pb.GetSessionIDPrev()
	msg.SessionIDNext = pb.GetSessionIDNext()
	msg.SessionIDMismatch = pb.GetSessionIDMismatch()
	msg.NotificationSession = pb.GetNotificationSession()
	msg.ContinuousSession = pb.GetContinuousSession()
	msg.SuppressResponse = pb.GetSuppressResponse()
	msg.Voltage100 = pb.GetVoltage100()
	msg.Temp100 = pb.GetTemp100()
	msg.Voltage1000 = pb.GetVoltage1000()
	msg.Temp1000 = pb.GetTemp1000()
	msg.UsageProvisioned = pb.GetUsageProvisioned()
	msg.UsageRcvdBytes = pb.GetUsageRcvdBytes()
	msg.UsageSentBytes = pb.GetUsageSentBytes()
	msg.UsageRcvdBytesSecondary = pb.GetUsageRcvdBytesSecondary()
	msg.UsageSentBytesSecondary = pb.GetUsageSentBytesSecondary()
	msg.UsageTCPSessions = pb.GetUsageTCPSessions()
	msg.UsageTLSSessions = pb.GetUsageTLSSessions()
	msg.UsageRcvdNotes = pb.GetUsageRcvdNotes()
	msg.UsageSentNotes = pb.GetUsageSentNotes()
	msg.HighPowerSecsTotal = pb.GetHighPowerSecsTotal()
	msg.HighPowerSecsData = pb.GetHighPowerSecsData()
	msg.HighPowerSecsGPS = pb.GetHighPowerSecsGPS()
	msg.HighPowerCyclesTotal = pb.GetHighPowerCyclesTotal()
	msg.HighPowerCyclesData = pb.GetHighPowerCyclesData()
	msg.HighPowerCyclesGPS = pb.GetHighPowerCyclesGPS()
	msg.CellID = pb.GetCellID()
	msg.MotionSecs = pb.GetMotionSecs()
	msg.MotionOrientation = pb.GetMotionOrientation()
	msg.SessionTrigger = pb.GetSessionTrigger()

	// Validate the PB
	binaryLengthExpected := pb.GetBytes1() + pb.GetBytes2() + pb.GetBytes3() + pb.GetBytes4()
	if binaryLength != binaryLengthExpected {
		err = fmt.Errorf("wire: protobuf length actual %d != expected %d", binaryLength, binaryLengthExpected)
		return
	}

	// Extract the binary
	if msg.nf == nil {
		msg.nf = &nf{}
	}
	bytesLen := int(pb.GetBytes1())
	if bytesLen != 0 {
		msg.nf.Notefile = &Notefile{}
		jdata, err2 := jsonDecompress(wire[wirebase : wirebase+bytesLen])
		if err2 != nil {
			err = err2
			return
		}
		err = note.JSONUnmarshal(jdata, msg.nf.Notefile)
		if err != nil {
			return
		}
		wirebase += bytesLen
	}
	bytesLen = int(pb.GetBytes2())
	if bytesLen != 0 {
		msg.nf.Notefiles = &map[string]*Notefile{}
		jdata, err2 := jsonDecompress(wire[wirebase : wirebase+bytesLen])
		if err2 != nil {
			err = err2
			return
		}
		err = note.JSONUnmarshal(jdata, msg.nf.Notefiles)
		if err != nil {
			return
		}
		wirebase += bytesLen
	}
	bytesLen = int(pb.GetBytes3())
	if bytesLen != 0 {
		msg.nf.Body = &map[string]interface{}{}
		jdata, err2 := jsonDecompress(wire[wirebase : wirebase+bytesLen])
		if err2 != nil {
			err = err2
			return
		}
		err = note.JSONUnmarshal(jdata, msg.nf.Body)
		if err != nil {
			return
		}
		wirebase += bytesLen
	}
	bytesLen = int(pb.GetBytes4())
	if bytesLen != 0 {
		payload := wire[wirebase : wirebase+bytesLen]
		msg.nf.Payload = &payload
		wirebase += bytesLen
	}

	// Done
	wirelen = wirebase
	return
}

// WireReadRequest reads a message from the specified reader
func WireReadRequest(conn net.Conn, waitIndefinitely bool) (bytesRead uint32, request []byte, err error) {
	var n int
	var version []byte

	// Set up for reading with a timeout
	timeoutDuration := 30 * time.Second
	rdconn := io.Reader(conn)

	// Read the payload buffer format
	for {
		var err2 error
		versionLen := 1
		version = make([]byte, versionLen)
		_ = conn.SetReadDeadline(time.Now().Add(timeoutDuration))
		n, err2 = rdconn.Read(version)
		if debugWireRead {
			if err2 == nil {
				logDebug(context.Background(), "\n\nrdVersion(%d) %d", len(version), n)
			}
		}
		if err2, ok := err2.(net.Error); ok && err2.Timeout() {
			if !waitIndefinitely {
				err = fmt.Errorf("wire read: " + note.ErrTimeout + " timeout on read")
				return
			}
			continue
		}
		if err2 == io.EOF {
			err = fmt.Errorf("wire read: " + note.ErrClosed + " connection closed")
			return
		}
		if err2 != nil {
			err = fmt.Errorf("wire read: can't read version: %s", err2)
			return
		}
		if n != versionLen {
			err = fmt.Errorf("wire read: insufficient data to read format: %d/%d", n, versionLen)
			return
		}
		bytesRead += uint32(n)
		break
	}

	// Process the version byte to determine the length of th header that follows
	isValidVersion, headerLength := wireProcessVersionByte(version[0])
	if !isValidVersion {
		err = fmt.Errorf("wire read: unrecognized protocol")
		return
	}

	// Read the header
	header := make([]byte, headerLength)
	_ = conn.SetReadDeadline(time.Now().Add(timeoutDuration))
	if debugWireRead {
		logDebug(context.Background(), "rdHeader(%d)", len(header))
	}
	n, err = io.ReadFull(rdconn, header)
	if debugWireRead {
		if err == nil {
			logDebug(context.Background(), "rdHeader(%d) %d", len(header), n)
		} else {
			logWarn(context.Background(), "rdHeader(%d) %d %s", len(header), n, err)
		}
	}
	if err != nil {
		err = fmt.Errorf("wire read: can't read %d-byte header: %s", headerLength, err)
		return
	}
	if n != headerLength {
		err = fmt.Errorf("wire read: insufficient data to read header: %d/%d", n, headerLength)
		return
	}
	bytesRead += uint32(n)

	// Process the header
	protobufLength, binaryLength, err := wireReadHeader(version[0], header)
	if err != nil {
		return
	}

	// Read the protocol buffer, if it's present
	var protobuf []byte
	if protobufLength != 0 {
		protobuf = make([]byte, protobufLength)
		_ = conn.SetReadDeadline(time.Now().Add(timeoutDuration))
		if debugWireRead {
			logDebug(context.Background(), "rdProtobuf(%d)", len(protobuf))
		}
		n, err = io.ReadFull(rdconn, protobuf)
		if debugWireRead {
			if err == nil {
				logDebug(context.Background(), "rdProtobuf(%d) %d", len(protobuf), n)
			} else {
				logWarn(context.Background(), "rdProtobuf(%d) %d %s", len(protobuf), n, err)
			}
		}
		if err != nil {
			err = fmt.Errorf("wire read: can't read %d-byte protobuf: %s", protobufLength, err)
			return
		}
		if n != int(protobufLength) {
			err = fmt.Errorf("wire read: insufficient data to read protobuf: %d/%d", n, protobufLength)
			return
		}
		bytesRead += uint32(n)
		// Before proceeding, bail if this is obviously an invalid protobuf
		pb := NotehubPB{}
		err = proto.Unmarshal(protobuf, &pb)
		if err != nil {
			err = fmt.Errorf("wire: protobuf validation: %s", err)
			return
		}
	}

	// Read the binary data, if it's present
	var binary []byte
	if binaryLength != 0 {
		binary = make([]byte, binaryLength)
		_ = conn.SetReadDeadline(time.Now().Add(timeoutDuration))
		if debugWireRead {
			logDebug(context.Background(), "rdBinary(%d)", len(binary))
		}
		n, err = io.ReadFull(rdconn, binary)
		if debugWireRead {
			if err == nil {
				logDebug(context.Background(), "rdBinary(%d) %d", len(binary), n)
			} else {
				logWarn(context.Background(), "rdBinary(%d) %d %s", len(binary), n, err)
			}
		}
		if err != nil {
			err = fmt.Errorf("wire read: can't read %d-byte binary data: %s", binaryLength, err)
			return
		}
		if n != int(binaryLength) {
			err = fmt.Errorf("wire read: insufficient data available to read binary data: %d/%d", n, binaryLength)
			return
		}
		bytesRead += uint32(n)
	}

	// Combine all that we've read
	request = append(version, header...)
	if protobufLength != 0 {
		request = append(request, protobuf...)
	}
	if binaryLength != 0 {
		request = append(request, binary...)
	}
	return
}

// PacketMaxData returns the largest amount of data allowed
func (h *PacketHandler) PacketMaxData(encrypted bool) (length int) {

	// Compute the length available
	length = int(h.Notecard.PacketMtu)

	// Packet flag byte
	length -= 1

	// ConnectionID
	if h.Notecard.PacketCidType != CidNone {
		length -= len(h.Notehub.Cid)
	}

	// If encrypted
	if encrypted {
		length -= 1 // MessagePort
		length -= ChaCha20Poly1305IvLen
		length -= ChaCha20Poly1305AuthTagLen
	} else {
		length -= 1 // MessagePort
	}

	// Done
	return length

}

// PacketToWire converts a Packet to wire format, which is wholly little-endian
// See notehub-defs.go for binary wire format of PB payload
func (h *PacketHandler) PacketToWire(payload Packet, secureData bool, downlinksPending bool) (msg []byte, err error) {

	// Determine whether or not this packet will be encrypted
	encrypted := secureData
	if !h.Notehub.MayEncrypt {
		encrypted = false
	}
	if h.Notehub.MustEncrypt {
		encrypted = true
	}

	// Bail if too much data
	if len(payload.Data) > h.PacketMaxData(encrypted) {
		err = fmt.Errorf("data length %d is greater than max allowed %d", len(payload.Data), h.PacketMaxData(encrypted))
		return
	}

	// Attempt to encode the data using Snappy, and only use the compressed data if it shrinks.
	// This doesn't give us extra room to pack data within the MTU because it's coming too late,
	// but this does save us over-the-air bytes and thus dollars.  Note that the data, but not
	// the port, is compressed.  There's no reason for this other than code flow.
	compressed := false
	if len(payload.Data) > 0 {
		data := snappy.Encode(nil, payload.Data)
		if len(data) < len(payload.Data) {
			payload.Data = data
			compressed = true
		}
	}

	// Construct the flag byte and insert it
	packetHeader := h.Notecard.PacketCidType & PacketTypeMask
	if encrypted {
		packetHeader |= PacketFlagEncrypted
	}
	if compressed {
		packetHeader |= PacketFlagCompressed
	}
	if downlinksPending {
		packetHeader |= PacketFlagDownlinksPending
	}
	msg = append(msg, packetHeader)

	// Append the connection ID
	if h.Notecard.PacketCidType != CidNone {
		msg = append(msg, h.Notehub.Cid...)
	}

	// If unencrypted, converting to wire is trivial
	if !encrypted {
		msg = append(msg, payload.MessagePort)
		msg = append(msg, payload.Data...)
		return
	}

	// Generate the cleartext by appending the data to the port
	cleartext := append([]byte{payload.MessagePort}, payload.Data...)

	// Encrypt, appending the MAC computed using the AuthTagPlaintext
	var aead cipher.AEAD
	aead, err = chacha20poly1305.New(h.Notecard.EncrKey)
	if err != nil {
		return
	}
	iv := make([]byte, ChaCha20Poly1305IvLen)
	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		return
	}
	ciphertext := aead.Seal(nil, iv, cleartext, []byte(ChaCha20Poly1305AuthTagPlaintext))

	// Append the IV, ciphertext, and authTag to the message, and we're done
	msg = append(msg, iv...)
	msg = append(msg, ciphertext...)
	return

}

// packetHeaderFromWire extracts the connection ID from the wire format
func PacketHeaderFromWire(wire []byte) (cid []byte, packetFlags byte, dataOffset int, err error) {

	if len(wire) < 1 {
		err = fmt.Errorf("packet: zero-length packet")
		return
	}

	// Get the packet flags
	packetFlags = wire[dataOffset]
	packetType := packetFlags & PacketTypeMask
	dataOffset++

	// Return the connection ID if it's in the header
	switch packetType {

	case CidNone:

	case CidRandom:
		if len(wire)-dataOffset < CidRandomLen {
			err = fmt.Errorf("packet: cid underrun")
			return
		}
		cid = wire[dataOffset : dataOffset+CidRandomLen]
		dataOffset += CidRandomLen

	default:
		err = fmt.Errorf("wire: packet: unknown CID type")
		return

	}

	// Done
	return

}

// PacketFromWire converts a request from wire format
func (h *PacketHandler) PacketFromWire(wire []byte) (msg Packet, err error) {

	off := 0
	left := len(wire)

	// Extract CID
	var dataOffset int
	var packetFlags byte
	msg.ConnectionID, packetFlags, dataOffset, err = PacketHeaderFromWire(wire)
	if err != nil {
		return
	}
	off += dataOffset
	left -= dataOffset

	// If unencrypted, converting from wire is trivial
	if (packetFlags & PacketFlagEncrypted) == 0 {
		msg.Data = wire[off:]

		// Extract the port
		if len(msg.Data) < 1 {
			err = fmt.Errorf("packet header trunc: port and data")
			return
		}
		msg.MessagePort = msg.Data[0]
		msg.Data = msg.Data[1:]

		// If the packet's data was compressed, decompress it
		if len(msg.Data) > 0 && (packetFlags&PacketFlagCompressed) != 0 {
			msg.Data, err = snappy.Decode(nil, msg.Data)
			if err != nil {
				return
			}
		}

		// Done
		return

	} else if h.Notecard.EncrAlg == "" || len(h.Notecard.EncrKey) == 0 {
		err = fmt.Errorf("packet: encrypted packet received in absence of a PSK")
		return
	}

	// Extract the IV
	if left < ChaCha20Poly1305IvLen {
		err = fmt.Errorf("packet header trunc: IV")
		return
	}
	iv := wire[off : off+ChaCha20Poly1305IvLen]
	off += ChaCha20Poly1305IvLen
	left -= ChaCha20Poly1305IvLen

	// Decrypt and verify auth tag
	ciphertext := wire[off:]
	var aead cipher.AEAD
	aead, err = chacha20poly1305.New(h.Notecard.EncrKey)
	if err != nil {
		return
	}
	var cleartext []byte
	cleartext, err = aead.Open(nil, iv, ciphertext, []byte(ChaCha20Poly1305AuthTagPlaintext))
	if err != nil {
		return
	}

	// Extract the port from the beginning of the decrypted data
	if len(cleartext) < 2 {
		err = fmt.Errorf("packet received too little data")
		return
	}
	msg.MessagePort = cleartext[0]
	msg.Data = cleartext[1:]

	// If the packet's data was compressed, decompress it
	if len(msg.Data) > 0 {
		if (packetFlags & PacketFlagCompressed) != 0 {
			msg.Data, err = snappy.Decode(nil, msg.Data)
			if err != nil {
				return
			}
		}
	}

	// Final validation
	if len(msg.Data) > h.PacketMaxData((packetFlags&PacketFlagEncrypted) != 0) {
		err = fmt.Errorf("packet received data too large (%d)", left)
		return
	}

	return

}
