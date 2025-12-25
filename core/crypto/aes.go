package crypto

import (
	"crypto/aes"
	"errors"
)

// EncryptECB 使用 AES ECB 模式加密，内部执行 PKCS7 填充。
func EncryptECB(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	padded := pkcs7Padding(plaintext, blockSize)
	ciphertext := make([]byte, len(padded))
	for bs, be := 0, blockSize; bs < len(padded); bs, be = bs+blockSize, be+blockSize {
		block.Encrypt(ciphertext[bs:be], padded[bs:be])
	}
	return ciphertext, nil
}

// DecryptECB 使用 AES ECB 模式解密，内部执行 PKCS7 去填充。
func DecryptECB(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%blockSize != 0 {
		return nil, errors.New("ciphertext size error")
	}
	plaintext := make([]byte, len(ciphertext))
	for bs, be := 0, blockSize; bs < len(ciphertext); bs, be = bs+blockSize, be+blockSize {
		block.Decrypt(plaintext[bs:be], ciphertext[bs:be])
	}
	return pkcs7Unpadding(plaintext, blockSize)
}

// EncryptHexECB 以 ECB 加密后返回十六进制字符串，出错返回空串。
func EncryptHexECB(key []byte, plaintext string) string {
	data, err := EncryptECB(key, []byte(plaintext))
	if err != nil {
		return ""
	}
	return encodeHex(data)
}

func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	for i := 0; i < padding; i++ {
		data = append(data, byte(padding))
	}
	return data
}

func pkcs7Unpadding(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid padding size")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errors.New("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if int(data[i]) != padding {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

func encodeHex(data []byte) string {
	const alphabet = "0123456789abcdef"
	dst := make([]byte, len(data)*2)
	for i, b := range data {
		dst[i*2] = alphabet[b>>4]
		dst[i*2+1] = alphabet[b&0x0f]
	}
	return string(dst)
}
