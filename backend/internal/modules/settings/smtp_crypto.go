package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const smtpCiphertextVersionPrefix = "v1:"

// ParseSMTPEncryptionKey 解析 SMTP_ENCRYPTION_KEY 环境变量原值。
// 空字符串表示应用未配置加密能力，返回 (nil, nil)，调用方据此判断加解密不可用而不是报错。
// 非空但不是合法的 base64 编码 32 字节值时返回错误；错误信息不包含原始 key 内容，
// 因为显式配置错误应尽早在启动时暴露，但不能把密钥原文写进日志或 panic 信息。
func ParseSMTPEncryptionKey(raw string) (cipher.AEAD, error) {
	if raw == "" {
		return nil, nil
	}
	keyBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("invalid SMTP_ENCRYPTION_KEY: not valid base64")
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("invalid SMTP_ENCRYPTION_KEY: expected 32 bytes after base64 decoding, got %d", len(keyBytes))
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, errors.New("invalid SMTP_ENCRYPTION_KEY: failed to initialize cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("invalid SMTP_ENCRYPTION_KEY: failed to initialize AEAD")
	}
	return gcm, nil
}

// smtpPasswordAAD 构造 AES-GCM 的 additional data，绑定 (userID, adminAccountID)，
// 防止某个 workspace 的密文被复制到另一个 workspace 后仍能被解密使用。
func smtpPasswordAAD(userID, adminAccountID string) []byte {
	return []byte(userID + "\x00" + adminAccountID)
}

// encryptSMTPPassword 用 AES-256-GCM 加密明文密码，输出固定格式 "v1:<base64(nonce+ciphertext)>"。
func encryptSMTPPassword(gcm cipher.AEAD, userID, adminAccountID, plaintext string) (string, error) {
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), smtpPasswordAAD(userID, adminAccountID))
	return smtpCiphertextVersionPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptSMTPPassword 解密已保存的密文。任何一步失败（版本前缀、base64、nonce 长度、认证标签、
// additional data 不匹配）都统一返回 ErrSMTPDecryptFailed，不区分具体子原因，
// 避免把解密失败伪装成或泄漏为 SMTP 认证失败的细节。
func decryptSMTPPassword(gcm cipher.AEAD, userID, adminAccountID, stored string) (string, error) {
	if !strings.HasPrefix(stored, smtpCiphertextVersionPrefix) {
		return "", ErrSMTPDecryptFailed
	}
	encoded := strings.TrimPrefix(stored, smtpCiphertextVersionPrefix)
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrSMTPDecryptFailed
	}
	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", ErrSMTPDecryptFailed
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, smtpPasswordAAD(userID, adminAccountID))
	if err != nil {
		return "", ErrSMTPDecryptFailed
	}
	return string(plaintext), nil
}
