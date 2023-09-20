// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// This package is a Go implementation of the smaz algorithm, renamed to "smazz" because
// of one modification: a zero byte '\0' is always stored at the beginning of encoded
// data, and if the encoded version of a the data would be longer than the original
// data, it is stored as-is without the '\0'.
//
// For further info and credit, see:
//   https://github.com/kjk/smaz
//   https://github.com/antirez/smaz

package notelib

import (
	"errors"
	"fmt"
)

// Codebook optimized for JSON templates
var SmazzCodeTemplate = [254]string{
	"{\"", "\"}", "0", "1", "2", "3", "4", "5", "6", "7",
	"8", "9", "a", "b", "c", "d", "e", "f", "g", "h",
	"i", "j", "k", "l", "m", "n", "o", "p", "q", "r",
	"s", "t", "u", "v", "w", "x", "y", "z", " ", "+",
	"-", "_", ".", "?", "[", "]", "\":{", "},\"", "\":", "}}",
	"{", "}", "[", "]", ":", ",", "\":true", "\":11", "\":12",
	"\":13", "\":14", "\":18", "\":21", "\":22", "\":23", "\":24 ", "\":12.1", "\":14.1", "\":18.1",
	"\":\"0\"", "\":\"", "\"", ",\"a", ",\"b", ",\"c", ",\"d", ",\"e", ",\"f", ",\"g",
	",\"h", ",\"i", ",\"j", ",\"k", ",\"l", ",\"m", ",\"n", ",\"o", ",\"p", ",\"q",
	",\"r", ",\"s", ",\"t", ",\"u", ",\"v", ",\"w", ",\"x", ",\"y", ",\"z", ",\"_",
	",\".", "_a", "_b", "_c", "_d", "_e", "_f", "_g ", "_h", "_i",
	"_j", "_k", "_l", "_m", "_n", "_o", "_p", "_q", "_r ", "_s",
	"_t", "_u", "_v", "_w", "_x", "_y", "_z", "ax", "max", "in",
	"min", "ecs", "temp", "ate", "one", "io", "ll", "tr", "ce", "la",
	"sec", "fo", "es", "hg", "net.", "com.", "org.", "\",\"", "not", "ie",
	"the", "and", "tha", "ent", "ing", "ion", "tio", "for", "nde", "has",
	"nce", "edt", "tis", "oft", "sth", "men", "th", "he", "in", "en",
	"nt", "re", "er", "an", "ti", "es", "on", "at", "se", "nd",
	"or", "ar", "al", "te", "co", "de", "to", "ra", "et", "ed",
	"it", "sa", "em", "ro", "le", "1.", "2.", "3.", "st", "ly",
	"ta", "ha", "off", "pe", "si", "chg", "ati", "wa", "ec", "our",
	"who", "rs", "ot", "un", "im", "nc", "ver", "ad", "we", "ee",
	"id", "cl", "ac", "il", "rt", "wi", "div", "ma", "ge", "my",
	"iv", "vi", "ay", "db", "qi", "qo", "qu", "_min", "_max", "stat",
	"ds", "vol", "cha", "us", "rg", "pm", "ne", "um", "ss", "false",
	"ns", "tu", "ol", "$",
}

// Codebook for smaz compatibility
var SmazzCodeDefault = [254]string{" ",
	"the", "e", "t", "a", "of", "o", "and", "i", "n", "s", "e ", "r", " th",
	" t", "in", "he", "th", "h", "he ", "to", "\r\n", "l", "s ", "d", " a", "an",
	"er", "c", " o", "d ", "on", " of", "re", "of ", "t ", ", ", "is", "u", "at",
	"   ", "n ", "or", "which", "f", "m", "as", "it", "that", "\n", "was", "en",
	"  ", " w", "es", " an", " i", "\r", "f ", "g", "p", "nd", " s", "nd ", "ed ",
	"w", "ed", "http://", "for", "te", "ing", "y ", "The", " c", "ti", "r ", "his",
	"st", " in", "ar", "nt", ",", " to", "y", "ng", " h", "with", "le", "al", "to ",
	"b", "ou", "be", "were", " b", "se", "o ", "ent", "ha", "ng ", "their", "\"",
	"hi", "from", " f", "in ", "de", "ion", "me", "v", ".", "ve", "all", "re ",
	"ri", "ro", "is ", "co", "f t", "are", "ea", ". ", "her", " m", "er ", " p",
	"es ", "by", "they", "di", "ra", "ic", "not", "s, ", "d t", "at ", "ce", "la",
	"h ", "ne", "as ", "tio", "on ", "n t", "io", "we", " a ", "om", ", a", "s o",
	"ur", "li", "ll", "ch", "had", "this", "e t", "g ", "e\r\n", " wh", "ere",
	" co", "e o", "a ", "us", " d", "ss", "\n\r\n", "\r\n\r", "=\"", " be", " e",
	"s a", "ma", "one", "t t", "or ", "but", "el", "so", "l ", "e s", "s,", "no",
	"ter", " wa", "iv", "ho", "e a", " r", "hat", "s t", "ns", "ch ", "wh", "tr",
	"ut", "/", "have", "ly ", "ta", " ha", " on", "tha", "-", " l", "ati", "en ",
	"pe", " re", "there", "ass", "si", " fo", "wa", "ec", "our", "who", "its", "z",
	"fo", "rs", ">", "ot", "un", "<", "im", "th ", "nc", "ate", "><", "ver", "ad",
	" we", "ly", "ee", " n", "id", " cl", "ac", "il", "</", "rt", " wi", "div",
	"e, ", " it", "whi", " ma", "ge", "x", "e c", "men", ".com",
}

type SmazzContext struct {
	codes    [254][]byte
	codeTrie *Trie
}

func Smazz(table [254]string) (ctx *SmazzContext) {
	ctx = &SmazzContext{}
	ctx.codeTrie = trieNew()
	ctx.codes = [254][]byte{}
	for i, code := range table {
		ctx.codes[i] = []byte(code)
		ctx.codeTrie.Put([]byte(code), i)
	}
	return
}

func appendSrc(dst, src []byte) []byte {
	// We can write a max of 255 continuous verbatim characters, because the
	// length of the continous verbatim section is represented by a single byte.
	for len(src) > 0 {
		left := len(src)
		if left == 1 {
			// 254 is code for a single verbatim byte
			dst = append(dst, byte(254))
			return append(dst, src[0])
		}
		toCopy := left
		if left > 255 {
			toCopy = 255
		}
		// 255 is code for a verbatim string. It is followed by a byte
		// containing the length of the string.
		dst = append(dst, byte(255))
		dst = append(dst, byte(toCopy))
		dst = append(dst, src[:toCopy]...)
		src = src[toCopy:]
	}
	return dst
}

// Encode returns the encoded form of src. The returned slice may be a sub-slice
// of dst if dst was large enough to hold the entire encoded block. Otherwise,
// a newly allocated slice will be returned. It is valid to pass a nil dst.
func (ctx *SmazzContext) Encode(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		return src, nil
	}
	if src[0] == 0 {
		return []byte{}, fmt.Errorf("can't encode a string beginning with 0")
	}
	orig := src
	dst = dst[:0]
	root := ctx.codeTrie.Root()
	currPos := 0
	nLeft := len(src)
	tmp := src
	code := 0
	prefixLen := 0
	for currPos < nLeft {
		node := root
		for i, c := range tmp {
			next := node.Walk(c)
			if next == nil {
				break
			}
			node = next
			if node.Terminal() {
				prefixLen = i + 1
				code = node.Val()
			}
		}
		if prefixLen == 0 {
			currPos++
			tmp = tmp[1:]
			continue
		}
		//		if trace && len(src[:currPos]) > 0 {
		//			fmt.Printf("append %d: '%s'\n", len(src[:currPos]), src[:currPos])
		//		}
		dst = appendSrc(dst, src[:currPos])
		//		if trace {
		//			fmt.Printf("code %d: '%s'\n", code, string(ctx.codes[code]))
		//		}
		dst = append(dst, byte(code))
		src = src[currPos+prefixLen:]
		currPos = 0
		nLeft = len(src)
		tmp = src
		prefixLen = 0
	}
	//	if trace && len(src) > 0 {
	//		fmt.Printf("append %d: '%s'\n", len(src), src)
	//	}
	dst = appendSrc(dst, src)
	// Prefix with 0 and return original source if it grew
	dst = append([]byte{0}, dst...)
	if len(dst) > len(orig) {
		return orig, nil
	}
	return dst, nil
}

// See if a buffer is known NOT to be Smazz-encoded
func (ctx *SmazzContext) Invalid(src []byte) bool {
	if len(src) == 0 {
		return true
	}
	if src[0] != 0 {
		return true
	}
	return false
}

// ErrCorrupt reports that the input is invalid.
var ErrCorrupt = errors.New("smazz: corrupt input")

// Decode returns the decoded form of src. The returned slice may be a sub-slice
// of dst if dst was large enough to hold the entire decoded block. Otherwise,
// a newly allocated slice will be returned. It is valid to pass a nil dst.
func (ctx *SmazzContext) Decode(dst, src []byte) ([]byte, error) {
	// If the start isn't '\0', return original string
	if len(src) == 0 {
		return src, nil
	}
	if src[0] != 0 {
		return src, nil
	}
	src = src[1:]
	// Smaz
	if cap(dst) < len(src) {
		dst = make([]byte, 0, len(src))
	}
	for len(src) > 0 {
		n := int(src[0])
		switch n {
		case 254: // Verbatim byte
			if len(src) < 2 {
				return nil, ErrCorrupt
			}
			dst = append(dst, src[1])
			src = src[2:]
		case 255: // Verbatim string
			if len(src) < 2 {
				return nil, ErrCorrupt
			}
			n = int(src[1])
			if len(src) < n+2 {
				return nil, ErrCorrupt
			}
			dst = append(dst, src[2:n+2]...)
			src = src[n+2:]
		default: // Look up encoded value
			d := ctx.codes[n]
			dst = append(dst, d...)
			src = src[1:]
		}
	}

	return dst, nil
}

// A Node represents a logical vertex in the trie structure.
type Node struct {
	branches [256]*Node
	val      int
	terminal bool
}

// A Trie is a a prefix tree.
type Trie struct {
	root *Node
}

// New construct a new, empty Trie ready for use.
func trieNew() *Trie {
	return &Trie{
		root: &Node{},
	}
}

// Put inserts the mapping k -> v into the Trie, overwriting any previous value. It returns true if the
// element was not previously in t.
func (t *Trie) Put(k []byte, v int) bool {
	n := t.root
	for _, c := range k {
		next := n.Walk(c)
		if next == nil {
			next = &Node{}
			n.branches[c] = next
		}
		n = next
	}
	n.val = v
	if n.terminal {
		return false
	}
	n.terminal = true
	return true
}

// Get the value corresponding to k in t, if any.
func (t *Trie) Get(k []byte) (v int, ok bool) {
	n := t.root
	for _, c := range k {
		next := n.Walk(c)
		if next == nil {
			return 0, false
		}
		n = next
	}
	if n.terminal {
		return n.val, true
	}
	return 0, false
}

// Root returns the root node of a Trie. A valid Trie (i.e., constructed with New), always has a non-nil root
// node.
func (t *Trie) Root() *Node { return t.root }

// Walk returns the node reached along edge c, if one exists. If node doesn't
// exist we return nil
func (n *Node) Walk(c byte) *Node {
	return n.branches[int(c)]
}

// Terminal indicates whether n is terminal in the trie (that is, whether the path from the root to n
// represents an element in the set). For instance, if the root node is terminal, then []byte{} is in the
// trie.
func (n *Node) Terminal() bool { return n.terminal }

// Val gives the value associated with this node. It panics if n is not terminal.
func (n *Node) Val() int {
	if !n.terminal {
		panic("Val called on non-terminal node")
	}
	return n.val
}
