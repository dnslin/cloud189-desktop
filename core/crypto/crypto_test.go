package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRSAEncryptAndDecrypt(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("生成 RSA 密钥失败: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("编码公钥失败: %v", err)
	}
	pub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	plaintext := []byte("cloud189")

	ciphertext, err := Encrypt(pub, plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	out, err := rsa.DecryptPKCS1v15(rand.Reader, priv, ciphertext)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("解密结果不一致，期望 %s 实际 %s", string(plaintext), string(out))
	}
}

func TestRSAInvalidKey(t *testing.T) {
	if _, err := Encrypt([]byte("bad-key"), []byte("data")); err == nil {
		t.Fatalf("无效公钥应返回错误")
	}
}

func TestAesEncryptDecryptECB(t *testing.T) {
	key := []byte("0123456789abcdef")
	plaintext := []byte("hello world")
	ciphertext, err := EncryptECB(key, plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	const expectedHex = "8169bed4ef49a8874559c5b200daade7"
	if hex.EncodeToString(ciphertext) != expectedHex {
		t.Fatalf("密文不匹配，期望 %s 实际 %s", expectedHex, hex.EncodeToString(ciphertext))
	}
	out, err := DecryptECB(key, ciphertext)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if string(out) != string(plaintext) {
		t.Fatalf("解密结果不一致，期望 %s 实际 %s", plaintext, out)
	}
}

func TestAesInvalidKeyLength(t *testing.T) {
	if _, err := EncryptECB([]byte("short"), []byte("data")); err == nil {
		t.Fatalf("短密钥应报错")
	}
}

func TestAesInvalidPadding(t *testing.T) {
	key := []byte("0123456789abcdef")
	ciphertext := make([]byte, aes.BlockSize)
	if _, err := DecryptECB(key, ciphertext); err == nil {
		t.Fatalf("无效填充应返回错误")
	}
}

func TestEncryptHexECB(t *testing.T) {
	key := []byte("0123456789abcdef")
	const expectedHex = "8169bed4ef49a8874559c5b200daade7"
	if out := EncryptHexECB(key, "hello world"); out != expectedHex {
		t.Fatalf("hex 输出不匹配，期望 %s 实际 %s", expectedHex, out)
	}
}

func TestHMACSign(t *testing.T) {
	const expected = "b34ceac4516ff23a143e61d79d0fa7a4fbe5f266"
	if Sign("hello", "key") != expected {
		t.Fatalf("字符串签名不匹配")
	}
	if hex.EncodeToString(SignBytes([]byte("hello"), []byte("key"))) != expected {
		t.Fatalf("字节签名不匹配")
	}
}

func TestMD5Digest(t *testing.T) {
	if DigestString("hello") != "5d41402abc4b2a76b9719d911017c592" {
		t.Fatalf("字符串 MD5 不匹配")
	}
	if DigestBytes([]byte("abc")) != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("字节 MD5 不匹配")
	}
}

func TestMD5File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	content := []byte("chunk-data")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	sum, err := DigestFile(path)
	if err != nil {
		t.Fatalf("文件计算失败: %v", err)
	}
	expected := md5.Sum(content)
	if sum != hex.EncodeToString(expected[:]) {
		t.Fatalf("文件 MD5 不匹配，期望 %s 实际 %s", hex.EncodeToString(expected[:]), sum)
	}
	if _, err := DigestFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatalf("不存在的文件应返回错误")
	}
}

func TestSecureRandomHex(t *testing.T) {
	out := SecureRandomHex(8)
	if len(out) != 16 {
		t.Fatalf("长度不匹配，期望 16 实际 %d", len(out))
	}
	if _, err := hex.DecodeString(out); err != nil {
		t.Fatalf("输出不是合法 hex: %v", err)
	}
}

func TestUUID(t *testing.T) {
	id := UUID()
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !re.MatchString(id) {
		t.Fatalf("UUID 格式错误: %s", id)
	}
}

func TestRandomString(t *testing.T) {
	out := RandomString(10, "abc")
	if len(out) != 10 {
		t.Fatalf("长度不匹配，期望 10 实际 %d", len(out))
	}
	for _, r := range out {
		if strings.IndexRune("abc", r) == -1 {
			t.Fatalf("字符 %c 不在字符集中", r)
		}
	}
	if RandomString(0, "abc") != "" {
		t.Fatalf("长度为 0 应返回空串")
	}
	if RandomString(5, "") != "" {
		t.Fatalf("空字符集应返回空串")
	}
}

func TestEncodeParams(t *testing.T) {
	params := map[string]string{"b": "2", "a": "1 2"}
	encoded := EncodeParams(params)
	items := strings.Split(encoded, "&")
	if len(items) != len(params) {
		t.Fatalf("编码数量不匹配")
	}
	decoded := make(map[string]string)
	for _, item := range items {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			t.Fatalf("编码格式错误: %s", item)
		}
		key, err := url.QueryUnescape(kv[0])
		if err != nil {
			t.Fatalf("解码 key 失败: %v", err)
		}
		val, err := url.QueryUnescape(kv[1])
		if err != nil {
			t.Fatalf("解码 value 失败: %v", err)
		}
		decoded[key] = val
	}
	if decoded["a"] != "1 2" || decoded["b"] != "2" {
		t.Fatalf("参数解码不匹配: %#v", decoded)
	}
}

func TestEncodeParamsSorted(t *testing.T) {
	params := map[string]string{"b": "2", "a": "1 2"}
	if out := EncodeParamsSorted(params); out != "a=1+2&b=2" {
		t.Fatalf("排序编码不匹配，期望 a=1+2&b=2 实际 %s", out)
	}
}
