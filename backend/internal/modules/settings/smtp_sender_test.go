package settings

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// generateTestCertificate 生成一张自签名测试证书 + 私钥，用于 fake SMTP server 的 TLS 监听。
// 测试通过把该证书的 CA 池注入 netSMTPSender.rootCAs 来校验证书，禁止使用 InsecureSkipVerify。
func generateTestCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"127.0.0.1"},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert := tls.Certificate{Certificate: [][]byte{derBytes}, PrivateKey: priv}
	parsedCert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsedCert)
	return cert, pool
}

func TestBuildSMTPTestMessageUsesCustomSubjectAndHTMLBody(t *testing.T) {
	message := string(buildSMTPTestMessage(smtpSendConfig{
		FromEmail:      "mailer@example.com",
		FromName:       "TransitHub",
		RecipientEmail: "recipient@example.com",
		Subject:        "TransitHub 限时活动",
		HTMLBody:       "<h1>自定义正文</h1>",
	}))
	if !strings.Contains(message, "Subject: =?UTF-8?q?TransitHub_=E9=99=90=E6=97=B6=E6=B4=BB=E5=8A=A8?=") {
		t.Fatalf("expected MIME encoded custom subject, got %s", message)
	}
	if !strings.Contains(message, "<h1>自定义正文</h1>") || strings.Contains(message, smtpTestHTMLBody) {
		t.Fatalf("expected custom html body only, got %s", message)
	}
}

func TestNetSMTPSenderRejectsCRLFInCustomSubjectBeforeDial(t *testing.T) {
	sender := newNetSMTPSender()
	err := sender.Send(context.Background(), smtpSendConfig{
		Host:           "127.0.0.1",
		Port:           1,
		TLSMode:        SmtpTLSModeStarttls,
		FromEmail:      "mailer@example.com",
		RecipientEmail: "recipient@example.com",
		Subject:        "bad\r\nBcc: victim@example.com",
		HTMLBody:       "<p>ok</p>",
	})
	if !errors.Is(err, ErrSMTPValidation) {
		t.Fatalf("expected CRLF validation before network dial, got %v", err)
	}
}

// fakeSMTPServerBehavior 控制 fake SMTP server 在各阶段的响应，用于覆盖认证失败/发送失败场景。
type fakeSMTPServerBehavior struct {
	requireAuth       bool
	rejectAuth        bool
	rejectRcpt        bool
	expectImplicitTLS bool
}

// fakeSMTPCapture 收集 fake SMTP server 观察到的关键协议要素，供测试断言：
//   - dataReceived：DATA 阶段实际收到的邮件正文，证明真的执行了 DATA 而不是只做 EHLO/STARTTLS。
//   - authPlainPayload：AUTH PLAIN 命令 base64 解码后的 "\x00authcid\x00password" 原始载荷，
//     用于断言含前导/尾随空格的密码原样到达服务端，而不是被 trim 后的版本。
//   - mailFrom / rcptTo：MAIL FROM / RCPT TO 命令的完整参数行，用于断言 envelope 使用裸地址。
//
// 每个通道都有缓冲，且协议本身是同步请求/响应模型，所以 Send() 返回后立即读取不会有竞态。
type fakeSMTPCapture struct {
	dataReceived     chan string
	authPlainPayload chan string
	mailFrom         chan string
	rcptTo           chan string
}

func newFakeSMTPCapture() *fakeSMTPCapture {
	return &fakeSMTPCapture{
		dataReceived:     make(chan string, 1),
		authPlainPayload: make(chan string, 1),
		mailFrom:         make(chan string, 1),
		rcptTo:           make(chan string, 1),
	}
}

// runFakeSMTPServer 起一个手写的最小 SMTP 状态机，支持 EHLO/STARTTLS/AUTH PLAIN/MAIL/RCPT/DATA/QUIT。
// implicit 模式下监听直接是 TLS；starttls 模式下先明文监听，收到 STARTTLS 命令后升级。
func runFakeSMTPServer(t *testing.T, cert tls.Certificate, implicit bool, behavior fakeSMTPServerBehavior) (addr string, capture *fakeSMTPCapture) {
	t.Helper()
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}

	var listener net.Listener
	var err error
	if implicit {
		listener, err = tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	} else {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	capture = newFakeSMTPCapture()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		serveFakeSMTPConn(conn, tlsConfig, behavior, capture)
	}()

	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().String(), capture
}

func serveFakeSMTPConn(conn net.Conn, tlsConfig *tls.Config, behavior fakeSMTPServerBehavior, capture *fakeSMTPCapture) {
	defer conn.Close()
	writeLine := func(c net.Conn, line string) { fmt.Fprintf(c, "%s\r\n", line) }
	trySend := func(ch chan<- string, value string) {
		select {
		case ch <- value:
		default:
		}
	}

	current := conn
	reader := bufio.NewReader(current)
	writeLine(current, "220 fake.smtp.test ESMTP")

	authenticated := false
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			exts := []string{"250-fake.smtp.test"}
			if !behavior.expectImplicitTLS {
				exts = append(exts, "250-STARTTLS")
			}
			exts = append(exts, "250 AUTH PLAIN")
			for _, e := range exts {
				writeLine(current, e)
			}
		case strings.HasPrefix(upper, "STARTTLS"):
			writeLine(current, "220 ready to start TLS")
			tlsConn := tls.Server(current, tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			current = tlsConn
			reader = bufio.NewReader(current)
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			if behavior.rejectAuth {
				writeLine(current, "535 authentication failed")
				continue
			}
			if fields := strings.Fields(line); len(fields) >= 3 {
				if decoded, err := base64.StdEncoding.DecodeString(fields[2]); err == nil {
					trySend(capture.authPlainPayload, string(decoded))
				}
			}
			authenticated = true
			writeLine(current, "235 authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM"):
			trySend(capture.mailFrom, line)
			writeLine(current, "250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			if behavior.rejectRcpt {
				writeLine(current, "550 no such user")
				continue
			}
			trySend(capture.rcptTo, line)
			writeLine(current, "250 OK")
		case strings.HasPrefix(upper, "DATA"):
			if behavior.requireAuth && !authenticated {
				writeLine(current, "530 authentication required")
				continue
			}
			writeLine(current, "354 start mail input")
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				trimmed := strings.TrimRight(dataLine, "\r\n")
				if trimmed == "." {
					break
				}
				dataLines = append(dataLines, trimmed)
			}
			writeLine(current, "250 OK: message accepted")
			trySend(capture.dataReceived, strings.Join(dataLines, "\n"))
		case strings.HasPrefix(upper, "QUIT"):
			writeLine(current, "221 bye")
			return
		default:
			writeLine(current, "500 unrecognized command")
		}
	}
}

func testSendConfig(host string, port int, tlsMode SmtpTLSMode) smtpSendConfig {
	return smtpSendConfig{
		Host:           host,
		Port:           port,
		FromEmail:      "mailer@example.com",
		FromName:       "TransitHub",
		RecipientEmail: "recipient@example.com",
		TLSMode:        tlsMode,
	}
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

func TestSMTPSenderStarttlsSendsDataSuccessfully(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, capture := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	if err := sender.Send(context.Background(), cfg); err != nil {
		t.Fatalf("expected successful send, got %v", err)
	}

	select {
	case data := <-capture.dataReceived:
		if !strings.Contains(data, "TransitHub SMTP test email was sent successfully") {
			t.Fatalf("expected test HTML body in DATA payload, got %q", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected DATA to be received by fake server, timed out")
	}
}

func TestSMTPSenderImplicitTLSSendsDataSuccessfully(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, capture := runFakeSMTPServer(t, cert, true, fakeSMTPServerBehavior{expectImplicitTLS: true})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeImplicit)
	if err := sender.Send(context.Background(), cfg); err != nil {
		t.Fatalf("expected successful send, got %v", err)
	}

	select {
	case <-capture.dataReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected DATA to be received by fake server, timed out")
	}
}

func TestSMTPSenderAuthenticationSuccess(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, capture := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{requireAuth: true})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	cfg.Username = "mailer@example.com"
	cfg.Password = "correct-password"
	if err := sender.Send(context.Background(), cfg); err != nil {
		t.Fatalf("expected successful authenticated send, got %v", err)
	}

	select {
	case <-capture.dataReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected DATA to be received by fake server, timed out")
	}
}

// TestSMTPSenderAuthPlainPreservesPasswordLeadingAndTrailingSpaces 覆盖整改要求：
// AUTH 密码含前导/尾随空格时，fake SMTP server 收到的必须是原文，而不是被 trim 后的版本。
func TestSMTPSenderAuthPlainPreservesPasswordLeadingAndTrailingSpaces(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, capture := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{requireAuth: true})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	cfg.Username = "mailer@example.com"
	cfg.Password = "  secret with spaces  "
	if err := sender.Send(context.Background(), cfg); err != nil {
		t.Fatalf("expected successful authenticated send, got %v", err)
	}

	select {
	case payload := <-capture.authPlainPayload:
		// AUTH PLAIN 载荷格式：\x00authcid\x00password。
		parts := strings.Split(payload, "\x00")
		if len(parts) != 3 {
			t.Fatalf("expected 3-part AUTH PLAIN payload, got %q", payload)
		}
		if parts[2] != "  secret with spaces  " {
			t.Fatalf("expected password bytes preserved exactly over the wire, got %q", parts[2])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected AUTH PLAIN payload to be captured, timed out")
	}
}

// TestSMTPSenderSkipsAuthWhenUsernameEmpty 覆盖整改要求：无认证 SMTP（username 为空）时，
// sender 绝不发送 AUTH 命令——不能静默降级成尝试认证或匿名重试认证。
func TestSMTPSenderSkipsAuthWhenUsernameEmpty(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, capture := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	cfg.Username = ""
	cfg.Password = ""
	if err := sender.Send(context.Background(), cfg); err != nil {
		t.Fatalf("expected unauthenticated send to succeed, got %v", err)
	}

	select {
	case <-capture.dataReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected DATA to be received by fake server, timed out")
	}
	select {
	case payload := <-capture.authPlainPayload:
		t.Fatalf("expected no AUTH command to be sent for unauthenticated SMTP, got payload %q", payload)
	default:
	}
}

func TestSMTPSenderRejectsPasswordWithoutUsernameBeforeDial(t *testing.T) {
	sender := &netSMTPSender{}
	cfg := testSendConfig("127.0.0.1", 1, SmtpTLSModeStarttls)
	cfg.Username = ""
	cfg.Password = "legacy-password"

	if err := sender.Send(context.Background(), cfg); err != ErrSMTPMissingConfig {
		t.Fatalf("expected ErrSMTPMissingConfig before dialing, got %v", err)
	}
}

// TestSMTPSenderUsesBareEnvelopeAddresses 覆盖整改要求：MAIL FROM / RCPT TO 必须是裸地址。
func TestSMTPSenderUsesBareEnvelopeAddresses(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, capture := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	if err := sender.Send(context.Background(), cfg); err != nil {
		t.Fatalf("expected successful send, got %v", err)
	}

	select {
	case mailFrom := <-capture.mailFrom:
		if !strings.Contains(mailFrom, "<"+cfg.FromEmail+">") {
			t.Fatalf("expected MAIL FROM to carry the bare from address, got %q", mailFrom)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected MAIL FROM to be captured, timed out")
	}
	select {
	case rcptTo := <-capture.rcptTo:
		if !strings.Contains(rcptTo, "<"+cfg.RecipientEmail+">") {
			t.Fatalf("expected RCPT TO to carry the bare recipient address, got %q", rcptTo)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected RCPT TO to be captured, timed out")
	}
}

func TestSMTPSenderAuthenticationFailureMapsToSendFailed(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, _ := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{rejectAuth: true})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	cfg.Username = "mailer@example.com"
	cfg.Password = "wrong-password"
	err := sender.Send(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected authentication failure to produce an error")
	}
	if !strings.Contains(err.Error(), ErrSMTPSendFailed.Error()) {
		t.Fatalf("expected error wrapping ErrSMTPSendFailed, got %v", err)
	}
}

func TestSMTPSenderRcptRejectionMapsToSendFailed(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, _ := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{rejectRcpt: true})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	err := sender.Send(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected RCPT rejection to produce an error")
	}
	if !strings.Contains(err.Error(), ErrSMTPSendFailed.Error()) {
		t.Fatalf("expected error wrapping ErrSMTPSendFailed, got %v", err)
	}
}

func TestSMTPSenderRejectsCRLFInHeaderValues(t *testing.T) {
	cert, pool := generateTestCertificate(t)
	addr, _ := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{})
	host, port := splitHostPort(t, addr)

	sender := &netSMTPSender{rootCAs: pool}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	cfg.FromName = "Evil\r\nBcc: attacker@example.com"
	if err := sender.Send(context.Background(), cfg); err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation for CRLF in header value, got %v", err)
	}
}

// TestSMTPSenderDoesNotSkipCertificateVerification 校验伪造证书对应的错误 CA 池会导致连接失败，
// 证明 netSMTPSender 没有通过 InsecureSkipVerify 之类的方式绕过证书校验。
func TestSMTPSenderDoesNotSkipCertificateVerification(t *testing.T) {
	cert, _ := generateTestCertificate(t)
	addr, _ := runFakeSMTPServer(t, cert, false, fakeSMTPServerBehavior{})
	host, port := splitHostPort(t, addr)

	// 故意不注入正确的 CA 池：系统根证书不会信任这张自签名测试证书，STARTTLS 应当失败。
	sender := &netSMTPSender{}
	cfg := testSendConfig(host, port, SmtpTLSModeStarttls)
	err := sender.Send(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected certificate verification failure without trusted CA pool")
	}
}
