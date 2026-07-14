package settings

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const (
	smtpDialTimeout    = 10 * time.Second
	smtpOverallTimeout = 15 * time.Second
	smtpTestSubject    = "TransitHub SMTP Test"
	smtpTestHTMLBody   = "<p>TransitHub SMTP test email was sent successfully.</p>"
)

// smtpSendConfig 承载测试邮件发送所需的全部参数，全部来自已保存配置和固定测试邮件内容；
// 不接受任何来自请求体的 SMTP host/port/password（由 service 层保证，见 TestSMTPEmail）。
type smtpSendConfig struct {
	Host           string
	Port           int
	Username       string
	Password       string
	TLSMode        SmtpTLSMode
	FromEmail      string
	FromName       string
	RecipientEmail string
	Subject        string
	HTMLBody       string
}

// smtpSender 是发送测试邮件的抽象接口，便于测试注入本地 fake SMTP server。
type smtpSender interface {
	Send(ctx context.Context, cfg smtpSendConfig) error
}

// netSMTPSender 是基于标准库 net/smtp 的生产实现。
// rootCAs 仅供包内测试注入自签根证书池；生产构造路径 newNetSMTPSender 保持为 nil，
// 即使用系统根证书，不允许跳过证书校验（InsecureSkipVerify 恒为 false）。
type netSMTPSender struct {
	rootCAs *x509.CertPool
}

// newNetSMTPSender 是生产环境使用的构造函数：不接受自定义根证书池。
func newNetSMTPSender() *netSMTPSender {
	return &netSMTPSender{}
}

func (s *netSMTPSender) Send(ctx context.Context, cfg smtpSendConfig) error {
	if err := validateSMTPHeaderValue(cfg.FromName); err != nil {
		return err
	}
	if err := validateSMTPHeaderValue(cfg.FromEmail); err != nil {
		return err
	}
	if err := validateSMTPHeaderValue(cfg.RecipientEmail); err != nil {
		return err
	}
	if err := validateSMTPHeaderValue(cfg.Subject); err != nil {
		return err
	}
	if cfg.Username == "" && cfg.Password != "" {
		return ErrSMTPMissingConfig
	}

	deadline := time.Now().Add(smtpOverallTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	// ServerName 固定来自 host，不从 from email 推导；两种 TLS 模式都强制 TLS 1.2+ 和系统根证书校验。
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.Host,
		RootCAs:            s.rootCAs,
		InsecureSkipVerify: false,
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: smtpDialTimeout}

	var conn net.Conn
	var err error
	if cfg.TLSMode == SmtpTLSModeImplicit {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("%w: dial: %v", ErrSMTPSendFailed, err)
	}
	defer conn.Close()
	// 连接级 deadline 覆盖 TLS 握手、SMTP 命令和整体发送，Go 标准库 net/smtp 没有原生 context 支持。
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("%w: set deadline: %v", ErrSMTPSendFailed, err)
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("%w: smtp handshake: %v", ErrSMTPSendFailed, err)
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("%w: EHLO: %v", ErrSMTPSendFailed, err)
	}

	if cfg.TLSMode == SmtpTLSModeStarttls {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("%w: server does not support STARTTLS", ErrSMTPSendFailed)
		}
		// client.StartTLS 内部会在升级成功后自动丢弃升级前的 capability 状态并重新执行 EHLO
		// （见 Go 标准库 net/smtp 实现），满足升级后才能 AUTH/MAIL/RCPT/DATA 的要求。
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("%w: STARTTLS: %v", ErrSMTPSendFailed, err)
		}
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("%w: authentication: %v", ErrSMTPSendFailed, err)
		}
	}

	if err := client.Mail(cfg.FromEmail); err != nil {
		return fmt.Errorf("%w: MAIL FROM: %v", ErrSMTPSendFailed, err)
	}
	if err := client.Rcpt(cfg.RecipientEmail); err != nil {
		return fmt.Errorf("%w: RCPT TO: %v", ErrSMTPSendFailed, err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("%w: DATA: %v", ErrSMTPSendFailed, err)
	}
	if _, err := writer.Write(buildSMTPTestMessage(cfg)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("%w: write message: %v", ErrSMTPSendFailed, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("%w: finalize message: %v", ErrSMTPSendFailed, err)
	}

	// QUIT 失败不影响邮件已经通过 DATA 阶段成功发送的事实，不作为失败处理。
	_ = client.Quit()
	return nil
}

// validateSMTPHeaderValue 拒绝任何包含 CRLF 的值，防止邮件头注入；直接拒绝而不是替换，
// 避免静默篡改用户配置。
func validateSMTPHeaderValue(value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return ErrSMTPValidation
	}
	return nil
}

// buildSMTPTestMessage 构造 HTML 邮件。Subject/HTMLBody 为空时回退到固定 SMTP 测试内容，
// 保持既有 /api/settings/smtp/test-email 的静态测试邮件合同不变。
// From/To 头使用 net/mail.Address 编码，Subject 使用 MIME encoded-word，避免手写不安全的头拼接。
func buildSMTPTestMessage(cfg smtpSendConfig) []byte {
	from := (&mail.Address{Name: cfg.FromName, Address: cfg.FromEmail}).String()
	to := (&mail.Address{Address: cfg.RecipientEmail}).String()
	subjectText := cfg.Subject
	if subjectText == "" {
		subjectText = smtpTestSubject
	}
	htmlBody := cfg.HTMLBody
	if htmlBody == "" {
		htmlBody = smtpTestHTMLBody
	}
	subject := mime.QEncoding.Encode("UTF-8", subjectText)

	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n")
	return []byte(b.String())
}
