// Package mailer sends transactional emails via SMTP.
// Config is loaded from the site_settings table at send time so changes
// take effect without a service restart.
package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	tmplInvite  = template.Must(template.ParseFS(templateFS, "templates/invite.html"))
	tmplTest    = template.Must(template.ParseFS(templateFS, "templates/test.html"))
	tmplVerify  = template.Must(template.ParseFS(templateFS, "templates/verify.html"))
)

// Config holds SMTP connection settings.
type Config struct {
	Host        string
	Port        string // default "587"
	Username    string
	Password    string
	FromAddress string
	FromName    string
	TLS         bool // true = SMTPS (port 465); false = STARTTLS
}

// Mailer sends email using the provided Config.
type Mailer struct {
	cfg Config
}

// New returns a Mailer. cfg.Port defaults to "587" when empty.
func New(cfg Config) *Mailer {
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	return &Mailer{cfg: cfg}
}

// Send delivers a plain-text + HTML email to a single recipient.
func (m *Mailer) Send(_ context.Context, to, subject, htmlBody string) error {
	from := m.cfg.FromAddress
	if m.cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", m.cfg.FromName, m.cfg.FromAddress)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/html; charset=\"UTF-8\"\r\n")
	fmt.Fprintf(&buf, "\r\n")
	buf.WriteString(htmlBody)

	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	if m.cfg.TLS {
		return m.sendTLS(addr, auth, m.cfg.FromAddress, to, buf.Bytes())
	}
	return smtp.SendMail(addr, auth, m.cfg.FromAddress, []string{to}, buf.Bytes())
}

func (m *Mailer) sendTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: m.cfg.Host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.Auth(auth); err != nil {
		return err
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	if err = c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

// SendInvite renders the invite email template and delivers it.
func (m *Mailer) SendInvite(ctx context.Context, to, inviteURL string) error {
	var buf bytes.Buffer
	if err := tmplInvite.Execute(&buf, map[string]string{
		"InviteURL": inviteURL,
		"To":        to,
	}); err != nil {
		return err
	}
	return m.Send(ctx, to, "You've been invited to Orb", buf.String())
}

// SendTest sends a test email to verify SMTP configuration.
func (m *Mailer) SendTest(ctx context.Context, to string) error {
	var buf bytes.Buffer
	if err := tmplTest.Execute(&buf, map[string]string{"To": to}); err != nil {
		return err
	}
	return m.Send(ctx, to, "Orb SMTP test", buf.String())
}

// SendVerification renders the email verification template and delivers it.
func (m *Mailer) SendVerification(ctx context.Context, to, username, verifyURL string) error {
	var buf bytes.Buffer
	if err := tmplVerify.Execute(&buf, map[string]string{
		"Username":  username,
		"VerifyURL": verifyURL,
	}); err != nil {
		return err
	}
	return m.Send(ctx, to, "Verify your Orb email address", buf.String())
}

// Validate checks that the minimum required fields are present.
func (m *Mailer) Validate() error {
	var missing []string
	if m.cfg.Host == "" {
		missing = append(missing, "host")
	}
	if m.cfg.FromAddress == "" {
		missing = append(missing, "from_address")
	}
	if _, err := strconv.Atoi(m.cfg.Port); err != nil {
		missing = append(missing, "port (must be a number)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("smtp: missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}
