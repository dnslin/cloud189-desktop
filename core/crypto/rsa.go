package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

// Encrypt 使用 RSA 公钥进行 PKCS1v15 加密。
func Encrypt(pubPEM []byte, data []byte) ([]byte, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return nil, errors.New("public key error")
	}
	pub, err := parsePublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return rsa.EncryptPKCS1v15(rand.Reader, pub, data)
}

func parsePublicKey(der []byte) (*rsa.PublicKey, error) {
	pub, err := x509.ParsePKIXPublicKey(der)
	if err == nil {
		if key, ok := pub.(*rsa.PublicKey); ok {
			return key, nil
		}
		return nil, errors.New("public key type error")
	}
	if pkcs1, err2 := x509.ParsePKCS1PublicKey(der); err2 == nil {
		return pkcs1, nil
	}
	return nil, err
}
