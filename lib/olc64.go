// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

const (
	codeMaxLength = 14
	codeModulus   = 22
	codeTable     = "23456789CFGHJMPQRVWX+"
)

// OLC truncation and expansion functions
// Truncation: Simple bit shifts to remove bytes
// Expansion: Smart reconstruction in OLCFromINT64Bytes function

// Simple truncation functions - just remove bytes
func OLC64ToOLC56(x int64) int64 { return x >> 8 }  // Remove 1 byte
func OLC64ToOLC48(x int64) int64 { return x >> 16 } // Remove 2 bytes
func OLC64ToOLC40(x int64) int64 { return x >> 24 } // Remove 3 bytes
func OLC64ToOLC32(x int64) int64 { return x >> 32 } // Remove 4 bytes

// Simple expansion functions - just shift back
func OLC56ToOLC64(x int64) int64 { return x << 8 }  // Restore 1 byte
func OLC48ToOLC64(x int64) int64 { return x << 16 } // Restore 2 bytes
func OLC40ToOLC64(x int64) int64 { return x << 24 } // Restore 3 bytes
func OLC32ToOLC64(x int64) int64 { return x << 32 } // Restore 4 bytes

// bytesToChars maps byte count to number of complete base-22 characters
var bytesToChars = map[int]int{
	8: 14, // Full OLC
	7: 12, // Remove 2 chars
	6: 10, // Remove 4 chars
	5: 8,  // Coarse grid only
	4: 7,  // Truncated coarse grid
}

// OLCFromINT64Bytes converts a truncated OLC code into a valid OLC string
// byteCount indicates how many bytes were actually stored (4, 5, 6, 7, or 8)
func OLCFromINT64Bytes(code int64, byteCount int) (olc string) {
	if code == 0 {
		return ""
	}

	// Determine how many characters we can reliably extract based on byte count
	maxChars, ok := bytesToChars[byteCount]
	if !ok {
		maxChars = 14 // default to full
	}

	// Extract all characters from the code (up to 14)
	// The truncated code will naturally have fewer significant digits
	chars := []byte{}
	tempCode := code
	for i := 0; i < codeMaxLength; i++ {
		c := tempCode % codeModulus
		if c != codeModulus-1 {
			chars = append([]byte{codeTable[c]}, chars...)
		}
		tempCode = tempCode / codeModulus
	}

	// Truncate to maxChars (we only trust this many characters)
	if len(chars) > maxChars {
		chars = chars[:maxChars]
	}

	// Remove any '+' that might have been encoded
	olcClean := ""
	for _, ch := range string(chars) {
		if ch != '+' {
			olcClean += string(ch)
		}
	}

	// Pad with 'G' to get the right length
	// For truncated values, pad at the end (fine precision)
	for len(olcClean) < 8 {
		olcClean += "G"
	}

	// Format the OLC with '+' separator
	if maxChars <= 8 {
		// Coarse grid only
		olc = olcClean[:8] + "+"
	} else {
		// Has fine grid characters
		olc = olcClean[:8] + "+"
		remaining := olcClean[8:]

		// Calculate target length after '+'
		charsAfterPlus := maxChars - 8
		targetLen := charsAfterPlus
		if targetLen > 6 {
			targetLen = 6 // Max 6 chars after '+'
		}

		// Pad remaining to target length
		for len(remaining) < targetLen {
			remaining += "G"
		}
		if len(remaining) > targetLen {
			remaining = remaining[:targetLen]
		}

		olc = olc + remaining
	}

	return olc
}

// OLCFromINT64 converts a 64-bit OLC code into a string (assumes 8 bytes)
func OLCFromINT64(code int64) (olc string) {
	if code > 0 {
		for i := 0; i < codeMaxLength; i++ {
			c := code % codeModulus
			if c != codeModulus-1 {
				olc = string([]byte(codeTable)[c]) + olc
			}
			code = code / codeModulus

		}

		// Ensure the OLC has a '+' separator at the correct position
		// Check if there's already a '+' in the string
		hasPlus := false
		for _, ch := range olc {
			if ch == '+' {
				hasPlus = true
				break
			}
		}

		// If no '+' found and we have at least some characters, add it at position 8
		if !hasPlus && len(olc) > 0 {
			if len(olc) >= 8 {
				// Insert '+' after position 8
				olc = olc[:8] + "+" + olc[8:]
			} else {
				// Pad with '0' to get to 8 characters, then add '+'
				for len(olc) < 8 {
					olc = olc + "0"
				}
				olc = olc + "+"
			}
		}
	}
	return
}

// OLCToINT64 converts an OLC string into a 64-bit code
func OLCToINT64(olc string) (result int64) {
	blen := len(olc)
	for i := 0; i < codeMaxLength; i++ {
		var c int64
		if i >= blen {
			c = codeModulus - 1
		} else {
			switch []byte(olc)[i] {
			case 50: // 2
				c = 0
			case 51: // 3
				c = 1
			case 52: // 4
				c = 2
			case 53: // 5
				c = 3
			case 54: // 6
				c = 4
			case 55: // 7
				c = 5
			case 56: // 8
				c = 6
			case 57: // 9
				c = 7
			case 67: // C
				fallthrough
			case 99: // c
				c = 8
			case 70: // F
				fallthrough
			case 102: // f
				c = 9
			case 71: // G
				fallthrough
			case 103: // g
				c = 10
			case 72: // H
				fallthrough
			case 104: // h
				c = 11
			case 74: // J
				fallthrough
			case 106: // j
				c = 12
			case 77: // M
				fallthrough
			case 109: // m
				c = 13
			case 80: // P
				fallthrough
			case 112: // p
				c = 14
			case 81: // Q
				fallthrough
			case 113: // q
				c = 15
			case 82: // R
				fallthrough
			case 114: // r
				c = 16
			case 86: // V
				fallthrough
			case 118: // v
				c = 17
			case 87: // W
				fallthrough
			case 119: // w
				c = 18
			case 88: // X
				fallthrough
			case 120: // x
				c = 19
			case 43: // +
				c = 20
			default:
				result = -(int64(i) + 1)
				return
			}
		}
		result = result*codeModulus + c
	}
	return
}
