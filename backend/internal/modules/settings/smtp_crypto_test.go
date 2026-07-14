package settings

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func validTestKey() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 32))
}

func TestParseSMTPEncryptionKeyEmptyIsUnavailableNotError(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey("")
	if err != nil {
		t.Fatalf("expected no error for empty key, got %v", err)
	}
	if gcm != nil {
		t.Fatalf("expected nil AEAD for empty key")
	}
}

func TestParseSMTPEncryptionKeyRejectsWhitespaceOnly(t *testing.T) {
	raw := "   	  "
	_, err := ParseSMTPEncryptionKey(raw)
	if err == nil {
		t.Fatalf("expected whitespace-only key to be rejected")
	}
	if strings.Contains(err.Error(), raw) {
		t.Fatalf("error must not contain raw key content, got %v", err)
	}
}

func TestParseSMTPEncryptionKeyValidBase64ThirtyTwoBytes(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("expected valid key to parse, got %v", err)
	}
	if gcm == nil {
		t.Fatalf("expected non-nil AEAD for valid key")
	}
}

func TestParseSMTPEncryptionKeyRejectsInvalidBase64(t *testing.T) {
	_, err := ParseSMTPEncryptionKey("not-valid-base64!!!")
	if err == nil {
		t.Fatalf("expected error for invalid base64")
	}
	if strings.Contains(err.Error(), "not-valid-base64") {
		t.Fatalf("error must not contain raw key content, got %v", err)
	}
}

func TestParseSMTPEncryptionKeyRejectsWrongLength(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 16))
	_, err := ParseSMTPEncryptionKey(shortKey)
	if err == nil {
		t.Fatalf("expected error for non-32-byte key")
	}
	if strings.Contains(err.Error(), shortKey) {
		t.Fatalf("error must not contain raw key content, got %v", err)
	}
}

func TestEncryptSMTPPasswordDoesNotContainPlaintext(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	plaintext := "super-secret-password"
	ciphertext, err := encryptSMTPPassword(gcm, "user-1", "account-1", plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if strings.Contains(ciphertext, plaintext) {
		t.Fatalf("ciphertext must not contain plaintext")
	}
	if !strings.HasPrefix(ciphertext, "v1:") {
		t.Fatalf("expected v1: prefix, got %s", ciphertext)
	}
}

func TestEncryptSMTPPasswordProducesDifferentCiphertextEachTime(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	first, err := encryptSMTPPassword(gcm, "user-1", "account-1", "same-password")
	if err != nil {
		t.Fatalf("encrypt first: %v", err)
	}
	second, err := encryptSMTPPassword(gcm, "user-1", "account-1", "same-password")
	if err != nil {
		t.Fatalf("encrypt second: %v", err)
	}
	if first == second {
		t.Fatalf("expected different ciphertext for repeated encryption of same plaintext")
	}
}

func TestDecryptSMTPPasswordRoundTrip(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	plaintext := "my-smtp-password"
	ciphertext, err := encryptSMTPPassword(gcm, "user-1", "account-1", plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := decryptSMTPPassword(gcm, "user-1", "account-1", ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected decrypted %q, got %q", plaintext, decrypted)
	}
}

func TestDecryptSMTPPasswordFailsWithWrongKey(t *testing.T) {
	gcm1, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("parse key1: %v", err)
	}
	otherKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x02}, 32))
	gcm2, err := ParseSMTPEncryptionKey(otherKey)
	if err != nil {
		t.Fatalf("parse key2: %v", err)
	}
	ciphertext, err := encryptSMTPPassword(gcm1, "user-1", "account-1", "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, err = decryptSMTPPassword(gcm2, "user-1", "account-1", ciphertext)
	if err != ErrSMTPDecryptFailed {
		t.Fatalf("expected ErrSMTPDecryptFailed for wrong key, got %v", err)
	}
}

func TestDecryptSMTPPasswordFailsWithCorruptedCiphertext(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	ciphertext, err := encryptSMTPPassword(gcm, "user-1", "account-1", "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	corrupted := ciphertext[:len(ciphertext)-4] + "abcd"
	_, err = decryptSMTPPassword(gcm, "user-1", "account-1", corrupted)
	if err != ErrSMTPDecryptFailed {
		t.Fatalf("expected ErrSMTPDecryptFailed for corrupted ciphertext, got %v", err)
	}
}

// TestDecryptSMTPPasswordFailsAcrossWorkspaces 校验 AAD 绑定 (userID, adminAccountID)：
// 同一密文换一个 workspace 解密必须失败，防止密文跨 workspace 复用。
func TestDecryptSMTPPasswordFailsAcrossWorkspaces(t *testing.T) {
	gcm, err := ParseSMTPEncryptionKey(validTestKey())
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	ciphertext, err := encryptSMTPPassword(gcm, "user-1", "account-1", "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, err = decryptSMTPPassword(gcm, "user-1", "account-2", ciphertext)
	if err != ErrSMTPDecryptFailed {
		t.Fatalf("expected ErrSMTPDecryptFailed across workspaces, got %v", err)
	}
}
