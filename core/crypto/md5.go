package crypto

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// DigestString 计算字符串的 MD5 十六进制值。
func DigestString(s string) string {
	sum, _ := digest(strings.NewReader(s))
	return sum
}

// DigestBytes 计算字节数据的 MD5 十六进制值。
func DigestBytes(data []byte) string {
	sum, _ := digest(bytes.NewReader(data))
	return sum
}

// DigestFile 计算文件内容的 MD5 十六进制值。
func DigestFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return digest(f)
}

func digest(r io.Reader) (string, error) {
	hash := md5.New()
	if _, err := io.Copy(hash, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
