// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

const codeMaxLength = 14
const codeModulus = 22
const codeTable = "23456789CFGHJMPQRVWX+"

// OLCFromINT64 converts a 64-bit OLC code into a string
func OLCFromINT64(code int64) (olc string) {
	if code > 0 {
		for i := 0; i < codeMaxLength; i++ {
			c := code % codeModulus
			if c != codeModulus-1 {
				olc = string([]byte(codeTable)[c]) + olc
			}
			code = code / codeModulus

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
