package settings

import (
	"errors"
	"fmt"
	"net/textproto"
	"testing"
)

func TestRecipientRejectedOnlyForAddressProblems(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"enhanced 5.1.1", &textproto.Error{Code: 550, Msg: "5.1.1 <a@example.com>: Recipient address rejected: User unknown"}, true},
		{"gmail multi-line", &textproto.Error{Code: 550, Msg: "5.1.1 The email account that you tried to reach does not exist."}, true},
		{"bad destination domain", &textproto.Error{Code: 554, Msg: "5.1.2 domain does not accept mail"}, true},
		{"relay denied is our problem", &textproto.Error{Code: 550, Msg: "5.7.1 Relaying denied"}, false},
		{"policy wording without address problem", &textproto.Error{Code: 554, Msg: "5.7.1 Recipient address rejected: Access denied"}, false},
		{"163 style without enhanced code", &textproto.Error{Code: 550, Msg: "User not found: a@163.com"}, true},
		{"qq style", &textproto.Error{Code: 550, Msg: "Mailbox not found or access denied"}, true},
		{"553 relay hosts list", &textproto.Error{Code: 553, Msg: "sorry, that domain isn't in my list of allowed rcpthosts"}, false},
		{"554 without phrase", &textproto.Error{Code: 554, Msg: "Transaction failed"}, false},
		{"temporary failure", &textproto.Error{Code: 450, Msg: "4.2.0 Mailbox busy"}, false},
		{"wrapped", fmt.Errorf("rcpt: %w", &textproto.Error{Code: 550, Msg: "5.1.1 no such user"}), true},
		{"not a protocol error", errors.New("connection reset"), false},
	}
	for _, tc := range cases {
		if got := recipientRejected(tc.err); got != tc.want {
			t.Errorf("%s: recipientRejected = %t, want %t", tc.name, got, tc.want)
		}
	}
	if !errors.Is(ErrSMTPRecipientRejected, ErrSMTPSendFailed) {
		t.Fatal("a rejected recipient must still count as a send failure for existing handlers")
	}
	if errors.Is(ErrSMTPSendFailed, ErrSMTPRecipientRejected) {
		t.Fatal("an ordinary send failure must not look like a rejected recipient")
	}
}
