package chain

// EncodeWithdrawDestination returns UTF-8 bytes of the normalized address
// (lowercase when not case-sensitive). Matches TS encodeWithdrawDestination /
// evmUtf8ToHex (hex of UTF-8), not a 20-byte pubkey decode.
func EncodeWithdrawDestination(address string, isCaseSensitive bool) []byte {
	normalized := address
	if !isCaseSensitive {
		normalized = toLowerASCII(address)
	}
	return []byte(normalized)
}

func toLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
