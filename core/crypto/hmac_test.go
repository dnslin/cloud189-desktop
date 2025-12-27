package crypto

import (
	"encoding/hex"
	"testing"
)

// TestSign_KnownVector 使用 RFC 向量验证 HMAC-SHA1 签名。
func TestSign_KnownVector(t *testing.T) {
	message := "The quick brown fox jumps over the lazy dog"
	key := "key"
	const expectedHex = "de7c9b85b8b78aa6bc8a7a36f70a90701c9db4d9"

	if sig := Sign(message, key); sig != expectedHex {
		t.Fatalf("字符串签名不匹配，期望 %s，实际 %s", expectedHex, sig)
	}

	raw := SignBytes([]byte(message), []byte(key))
	if hex.EncodeToString(raw) != expectedHex {
		t.Fatalf("字节签名不匹配，期望 %s，实际 %s", expectedHex, hex.EncodeToString(raw))
	}
}
