package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"testing"
)

// TestEncryptDecrypt_RSA 使用包装后的公钥验证 RSA 加解密。
func TestEncryptDecrypt_RSA(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 私钥失败: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("公钥序列化失败: %v", err)
	}
	pemData := WrapRSAPubKey(base64.StdEncoding.EncodeToString(der))

	plaintext := []byte("cloud189 rsa encrypt test")
	ciphertext, err := Encrypt(pemData, plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	decrypted, err := rsa.DecryptPKCS1v15(rand.Reader, priv, ciphertext)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("解密结果不一致，期望 %q，实际 %q", plaintext, decrypted)
	}
}
