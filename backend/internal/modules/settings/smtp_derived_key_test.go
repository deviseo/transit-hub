package settings

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// 没配 SMTP_ENCRYPTION_KEY 时用凭据密钥派生的密钥：能保存密码，发信时能解开。
// 之后再配上 SMTP_ENCRYPTION_KEY，以前保存的密码照样能解开，新保存的用 SMTP_ENCRYPTION_KEY。
func TestDerivedSMTPKeyWorksAloneAndKeepsOldPasswordsReadable(t *testing.T) {
	repo := newFakeSMTPRepository()
	resolver := &fakeAdminAccountResolver{id: "acct-1"}
	derived := bytes.Repeat([]byte{0x11}, 32)
	ctx := context.Background()

	sender := &fakeSMTPSender{}
	derivedOnly := &Service{smtpRepo: repo, accounts: resolver, smtpSender: sender}
	if err := derivedOnly.SetSMTPEncryptionKey(""); err != nil {
		t.Fatal(err)
	}
	if err := derivedOnly.SetDerivedSMTPKey(derived); err != nil {
		t.Fatal(err)
	}
	input := validSaveInput()
	input.Password = "super-secret"
	if _, err := derivedOnly.SaveSMTPSettings(ctx, "user-1", input); err != nil {
		t.Fatalf("saving a password must work without SMTP_ENCRYPTION_KEY: %v", err)
	}
	if err := derivedOnly.TestSMTPEmail(ctx, "user-1", "to@example.com"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0].Password != "super-secret" {
		t.Fatalf("the saved password should decrypt for sending: %+v", sender.sent)
	}

	sender = &fakeSMTPSender{}
	withEnv := &Service{smtpRepo: repo, accounts: resolver, smtpSender: sender}
	if err := withEnv.SetSMTPEncryptionKey(validTestKey()); err != nil {
		t.Fatal(err)
	}
	if err := withEnv.SetDerivedSMTPKey(derived); err != nil {
		t.Fatal(err)
	}
	if err := withEnv.TestSMTPEmail(ctx, "user-1", "to@example.com"); err != nil || sender.sent[0].Password != "super-secret" {
		t.Fatalf("a password saved with the derived key must stay readable after SMTP_ENCRYPTION_KEY is set: %v", err)
	}

	input.Password = "new-secret"
	if _, err := withEnv.SaveSMTPSettings(ctx, "user-1", input); err != nil {
		t.Fatal(err)
	}
	if err := withEnv.TestSMTPEmail(ctx, "user-1", "to@example.com"); err != nil || sender.sent[1].Password != "new-secret" {
		t.Fatalf("new passwords use SMTP_ENCRYPTION_KEY: %v", err)
	}
	if err := derivedOnly.TestSMTPEmail(ctx, "user-1", "to@example.com"); !errors.Is(err, ErrSMTPDecryptFailed) {
		t.Fatalf("the derived key alone must not read a password encrypted with SMTP_ENCRYPTION_KEY: %v", err)
	}
}

func TestSetDerivedSMTPKeyRejectsWrongLength(t *testing.T) {
	if err := (&Service{}).SetDerivedSMTPKey([]byte("short")); err == nil {
		t.Fatal("a derived key must be 32 bytes")
	}
}
