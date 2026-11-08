// Package secretbox 加密保存在数据库和 Redis 里的登录凭据：站点会话、token、Admin Key、通知机器人的密钥、
// 代理密码等。数据库或备份泄露时，没有密钥就拿不到这些凭据。
//
// 用 AES-256-GCM，密文格式是 "enc:v1:<base64(nonce + 密文)>"。additional data 绑定凭据所在的位置
// （哪张表、哪个用户），复制到别的位置的密文解不开。
//
// 加密之前保存的明文照样能读，下次保存时改成加密；启动后各模块再把剩下的明文批量改成加密。
//
// 密钥来源（Load）：环境变量 CREDENTIAL_ENCRYPTION_KEY（base64 编码的 32 字节）优先；没有时用数据目录里的
// credential.key，第一次启动时自动生成。数据目录需要是持久卷，密钥丢了已保存的凭据就读不出来，只能重新登录。
package secretbox

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// KeyEnv 是指定密钥的环境变量（base64 编码的 32 字节）。
	KeyEnv = "CREDENTIAL_ENCRYPTION_KEY"
	// KeyFileName 是数据目录里保存自动生成密钥的文件。
	KeyFileName = "credential.key"

	prefix  = "enc:v1:"
	keySize = 32
)

var (
	// ErrUndecryptable：密文解不开（密钥换了、密文被改过或放错了位置）。
	ErrUndecryptable = errors.New("secretbox: cannot decrypt the stored credential (wrong key or corrupted data)")
	// ErrNoKey：保存的是密文，但当前没有密钥。
	ErrNoKey = errors.New("secretbox: the stored credential is encrypted but no key is configured")
)

// Box 用一把密钥加解密。nil 的 *Box 表示不加密：Seal 原样返回明文，Open 只能读明文。
type Box struct {
	aead cipher.AEAD
	id   string
	// secret 只用来派生给别的用途的子密钥（DeriveKey），不直接拿去加密别的东西。
	secret []byte
}

// New 用 32 字节的密钥创建 Box。
func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key)
	return &Box{aead: aead, id: hex.EncodeToString(sum[:4]), secret: append([]byte(nil), key...)}, nil
}

// DeriveKey 从这把密钥派生一把 32 字节的子密钥给别的用途用（HKDF-SHA256，purpose 区分用途，
// 不同用途得到互不相关的密钥）。同一把密钥、同一个 purpose 每次得到同一把子密钥。没有密钥时返回 ErrNoKey。
func (b *Box) DeriveKey(purpose string) ([]byte, error) {
	if b == nil {
		return nil, ErrNoKey
	}
	return hkdf.Key(sha256.New, b.secret, nil, "transithub/secretbox/"+purpose, keySize)
}

// ID 是密钥的短指纹（只用于日志，看两次启动用的是不是同一把密钥），不泄露密钥本身。
func (b *Box) ID() string {
	if b == nil {
		return ""
	}
	return b.id
}

// IsSealed 判断保存的值是不是本包加密的密文。
func IsSealed(value string) bool {
	return strings.HasPrefix(value, prefix)
}

// Seal 加密 plaintext；aad 是凭据所在的位置，解密时必须一致。没有密钥时原样返回明文。
func (b *Box) Seal(plaintext []byte, aad string) (string, error) {
	if b == nil {
		return string(plaintext), nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nonce, nonce, plaintext, []byte(aad))
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open 解密 Seal 的结果。不是密文（加密前保存的明文）时原样返回。
func (b *Box) Open(stored string, aad string) ([]byte, error) {
	if !IsSealed(stored) {
		return []byte(stored), nil
	}
	if b == nil {
		return nil, ErrNoKey
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil || len(sealed) < b.aead.NonceSize() {
		return nil, ErrUndecryptable
	}
	nonce, ciphertext := sealed[:b.aead.NonceSize()], sealed[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return nil, ErrUndecryptable
	}
	return plaintext, nil
}

// SealJSON 用于 jsonb 列：把 JSON 加密后存成一个 JSON 字符串。没有密钥时原样返回。
func (b *Box) SealJSON(plainJSON []byte, aad string) ([]byte, error) {
	if b == nil {
		return plainJSON, nil
	}
	sealed, err := b.Seal(plainJSON, aad)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sealed)
}

// OpenJSON 读取 SealJSON 存进 jsonb 列的值；加密前保存的 JSON（对象等）原样返回。
func (b *Box) OpenJSON(stored []byte, aad string) ([]byte, error) {
	trimmed := bytes.TrimSpace(stored)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return stored, nil
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err != nil || !IsSealed(text) {
		return stored, nil
	}
	return b.Open(text, aad)
}

// Load 取得加密密钥：环境变量的值 envValue 优先；为空时读数据目录里的 credential.key，没有就生成一个。
// source 说明密钥从哪来（写日志用）。
//
// 环境变量或密钥文件的内容不合法时返回错误（宁可启动失败，也不能换一把新密钥让已保存的凭据全部读不出来）。
// 数据目录不可写、又没有设置环境变量时返回 (nil, 原因, nil)：凭据不加密保存。
func Load(envValue string, dataDir string) (box *Box, source string, err error) {
	if value := strings.TrimSpace(envValue); value != "" {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != keySize {
			return nil, "", fmt.Errorf("invalid %s: expected base64 of %d bytes", KeyEnv, keySize)
		}
		box, err := New(key)
		return box, "environment variable " + KeyEnv, err
	}
	if strings.TrimSpace(dataDir) == "" {
		return nil, "no data directory and no " + KeyEnv, nil
	}
	path := filepath.Join(dataDir, KeyFileName)
	if box, err := readKeyFile(path); err == nil {
		return box, "key file " + path, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	box, err = generateKeyFile(path)
	if errors.Is(err, fs.ErrExist) {
		// 同时有别的进程刚生成了：用它的。
		box, err = readKeyFile(path)
		return box, "key file " + path, err
	}
	if err != nil {
		return nil, fmt.Sprintf("cannot create %s (%v) and no %s", path, err, KeyEnv), nil
	}
	return box, "new key file " + path, nil
}

func readKeyFile(path string) (*Box, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != keySize {
		return nil, fmt.Errorf("key file %s is damaged: expected base64 of %d bytes", path, keySize)
	}
	return New(key)
}

func generateKeyFile(path string) (*Box, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return New(key)
}

var reported sync.Map

// LogUnreadable 记录读不出来的凭据（同一位置每个进程只记一次）。调用方按「没有保存凭据」处理，
// 用户重新登录后就会用当前密钥重新保存。
func LogUnreadable(location string, err error) {
	if _, seen := reported.LoadOrStore(location, true); seen {
		return
	}
	log.Printf("[credentials] cannot read the saved credential at %s: %v. Log in again to replace it. "+
		"If the key changed (for example the data directory is not a persistent volume), restore the old %s or set %s.",
		location, err, KeyFileName, KeyEnv)
}
