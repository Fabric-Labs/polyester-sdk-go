// Package ethaddr validates hex Ethereum addresses and writes them in their
// EIP-55 checksummed form without linking go-ethereum, so packages that only
// carry addresses (such as environment) keep the client binary small.
package ethaddr

import (
	"strings"

	"golang.org/x/crypto/sha3"
)

// hexLen is the number of hex digits in a 20-byte address.
const hexLen = 40

// IsHex reports whether s is a 20-byte address in hex: 40 hex digits of any
// case, optionally prefixed with 0x or 0X. It does not verify a mixed-case
// checksum, the same rule as go-ethereum's common.IsHexAddress.
func IsHex(s string) bool {
	s = trimPrefix(s)
	if len(s) != hexLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

// Checksum returns the EIP-55 checksummed form of s, prefixed with 0x, the
// same string go-ethereum's common.HexToAddress(s).Hex() writes. ok is false
// when s is not an address IsHex accepts.
func Checksum(s string) (checksummed string, ok bool) {
	if !IsHex(s) {
		return "", false
	}
	lower := strings.ToLower(trimPrefix(s))
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(lower)) // a hash.Hash write never fails
	digest := hash.Sum(nil)

	out := make([]byte, 2+hexLen)
	out[0], out[1] = '0', 'x'
	for i := 0; i < hexLen; i++ {
		c := lower[i]
		nibble := digest[i/2]
		if i%2 == 0 {
			nibble >>= 4
		} else {
			nibble &= 0x0f
		}
		if c >= 'a' && nibble >= 8 {
			c -= 'a' - 'A'
		}
		out[2+i] = c
	}
	return string(out), true
}

func trimPrefix(s string) string {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return s[2:]
	}
	return s
}

func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}
