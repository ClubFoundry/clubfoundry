package auth

import "encoding/hex"

// isValidTokenFormat accepts exactly 64 lowercase hexadecimal characters.
func isValidTokenFormat(s string) bool {
	if len(s) != tokenLength*2 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
