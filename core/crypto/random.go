package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
)

// SecureRandomHex 生成指定字节长度的安全随机十六进制字符串。
func SecureRandomHex(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return encodeHex(b)
}

// UUID 生成版本 4 的随机 UUID。
func UUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%04x%08x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		binary.BigEndian.Uint16(b[10:12]),
		binary.BigEndian.Uint32(b[12:16]),
	)
}

// RandomString 根据字符集生成指定位数的随机字符串。
func RandomString(n int, charset string) string {
	if n <= 0 || len(charset) == 0 {
		return ""
	}
	max := big.NewInt(int64(len(charset)))
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return ""
		}
		buf[i] = charset[v.Int64()]
	}
	return string(buf)
}
