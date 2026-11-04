package secretbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testBox(t *testing.T, fill byte) *Box {
	t.Helper()
	box, err := New(bytes.Repeat([]byte{fill}, keySize))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestSealAndOpen(t *testing.T) {
	box := testBox(t, 1)
	sealed, err := box.Seal([]byte(`{"token":"secret"}`), "my_site_states/u1")
	if err != nil {
		t.Fatal(err)
	}
	if !IsSealed(sealed) || strings.Contains(sealed, "secret") {
		t.Fatalf("not encrypted: %q", sealed)
	}
	again, _ := box.Seal([]byte(`{"token":"secret"}`), "my_site_states/u1")
	if again == sealed {
		t.Fatalf("each seal must use a fresh nonce")
	}
	plain, err := box.Open(sealed, "my_site_states/u1")
	if err != nil || string(plain) != `{"token":"secret"}` {
		t.Fatalf("open: %q %v", plain, err)
	}
	// 复制到别的位置、换了密钥、被改过：都解不开。
	if _, err := box.Open(sealed, "my_site_states/u2"); !errors.Is(err, ErrUndecryptable) {
		t.Fatalf("other location: %v", err)
	}
	if _, err := testBox(t, 2).Open(sealed, "my_site_states/u1"); !errors.Is(err, ErrUndecryptable) {
		t.Fatalf("other key: %v", err)
	}
	tampered := sealed[:len(sealed)-4] + "AAAA"
	if _, err := box.Open(tampered, "my_site_states/u1"); !errors.Is(err, ErrUndecryptable) {
		t.Fatalf("tampered: %v", err)
	}
	if _, err := (*Box)(nil).Open(sealed, "my_site_states/u1"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key: %v", err)
	}
}

func TestPlaintextFromBeforeEncryptionStillReads(t *testing.T) {
	box := testBox(t, 1)
	if plain, err := box.Open(`{"token":"old"}`, "x"); err != nil || string(plain) != `{"token":"old"}` {
		t.Fatalf("legacy value: %q %v", plain, err)
	}
	if plain, err := box.OpenJSON([]byte(`{"token":"old"}`), "x"); err != nil || string(plain) != `{"token":"old"}` {
		t.Fatalf("legacy json: %q %v", plain, err)
	}
	// 普通的 JSON 字符串（不是密文）也原样返回。
	if plain, err := box.OpenJSON([]byte(`"just text"`), "x"); err != nil || string(plain) != `"just text"` {
		t.Fatalf("plain json string: %q %v", plain, err)
	}
}

func TestJSONColumns(t *testing.T) {
	box := testBox(t, 1)
	stored, err := box.SealJSON([]byte(`{"accessToken":"abc"}`), "upstream_sites/s1")
	if err != nil {
		t.Fatal(err)
	}
	var text string
	if err := json.Unmarshal(stored, &text); err != nil || !IsSealed(text) {
		t.Fatalf("jsonb value should be a JSON string holding the ciphertext: %s", stored)
	}
	plain, err := box.OpenJSON(stored, "upstream_sites/s1")
	if err != nil || string(plain) != `{"accessToken":"abc"}` {
		t.Fatalf("open json: %q %v", plain, err)
	}
	// 没有密钥：保存明文，读明文。
	var none *Box
	if stored, _ := none.SealJSON([]byte(`{"a":1}`), "x"); string(stored) != `{"a":1}` {
		t.Fatalf("no key should store plaintext: %s", stored)
	}
}

func TestLoadPrefersTheEnvironment(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, keySize))
	dir := t.TempDir()
	box, source, err := Load(key, dir)
	if err != nil || box == nil || !strings.Contains(source, KeyEnv) {
		t.Fatalf("env key: %v %q %v", box, source, err)
	}
	if _, err := os.Stat(filepath.Join(dir, KeyFileName)); !os.IsNotExist(err) {
		t.Fatalf("no key file should be written when the environment provides the key")
	}
	for _, bad := range []string{"not-base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, _, err := Load(bad, dir); err == nil {
			t.Fatalf("invalid env key %q should fail", bad)
		}
	}
}

func TestLoadGeneratesAndReusesTheKeyFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	first, source, err := Load("", dir)
	if err != nil || first == nil || !strings.HasPrefix(source, "new key file") {
		t.Fatalf("generate: %v %q %v", first, source, err)
	}
	info, err := os.Stat(filepath.Join(dir, KeyFileName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("key file should be private, mode %v", info.Mode().Perm())
	}
	second, source, err := Load("", dir)
	if err != nil || second.ID() != first.ID() || !strings.HasPrefix(source, "key file") {
		t.Fatalf("reuse: %v %q %v", second, source, err)
	}
	sealed, _ := first.Seal([]byte("x"), "a")
	if plain, err := second.Open(sealed, "a"); err != nil || string(plain) != "x" {
		t.Fatalf("the reloaded key must open old values: %v", err)
	}
	// 密钥文件坏了：报错，不能悄悄换一把新密钥。
	if err := os.WriteFile(filepath.Join(dir, KeyFileName), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load("", dir); err == nil {
		t.Fatalf("a damaged key file must not be replaced silently")
	}
}

func TestLoadWithoutAWritableDirectoryDisablesEncryption(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission checks need a non-root unix user")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	box, reason, err := Load("", filepath.Join(dir, "data"))
	if err != nil || box != nil || reason == "" {
		t.Fatalf("expected no box with a reason, got %v %q %v", box, reason, err)
	}
}
