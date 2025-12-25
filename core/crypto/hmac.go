package crypto

import (
	"crypto/hmac"
	"crypto/sha1"
)

// Sign 计算字符串的 HMAC-SHA1 十六进制签名。
func Sign(message, key string) string {
	return encodeHex(SignBytes([]byte(message), []byte(key)))
}

// SignBytes 计算原始 HMAC-SHA1 摘要。
func SignBytes(message, key []byte) []byte {
	mac := hmac.New(sha1.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}
