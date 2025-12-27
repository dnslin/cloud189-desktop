package crypto

import (
	"encoding/hex"
	"testing"
)

// TestEncryptDecryptECB_KnownVector 使用已知向量验证 ECB 加解密逻辑。
func TestEncryptDecryptECB_KnownVector(t *testing.T) {
	key := []byte("0123456789abcdef")
	plaintext := []byte("hello cloud189")

	ciphertext, err := EncryptECB(key, plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	gotHex := hex.EncodeToString(ciphertext)
	const expectedHex = "0f4e8362ce77bf92418b34633110d400"
	if gotHex != expectedHex {
		t.Fatalf("密文不匹配，期望 %s，实际 %s", expectedHex, gotHex)
	}

	decrypted, err := DecryptECB(key, ciphertext)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("解密结果不一致，期望 %q，实际 %q", plaintext, decrypted)
	}

	hexOutput := EncryptHexECB(key, string(plaintext))
	if hexOutput != expectedHex {
		t.Fatalf("十六进制输出不一致，期望 %s，实际 %s", expectedHex, hexOutput)
	}
}
