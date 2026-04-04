package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailConnector provides email integration for receiving and sending messages via SMTP/IMAP.
// This is a simplified implementation using SMTP for sending only.
// For full bidirectional email integration, an IMAP client would be needed.
type EmailConnector struct {
	smtpHost     string
	smtpPort     int
	username     string
	password     string
	fromAddr     string
	imapHost     string // Optional: for receiving emails
	imapPort     int
	httpClient   *smtp.Client
	messageChan  chan *Message
	stopChan     chan struct{}
	logger       *slog.Logger
	pollInterval time.Duration
}

// NewEmailConnector creates a new Email connector.
func NewEmailConnector(smtpHost string, smtpPort int, username, password, fromAddr string, logger *slog.Logger) *EmailConnector {
	if logger == nil {
		logger = slog.Default()
	}

	return &EmailConnector{
		smtpHost:     smtpHost,
		smtpPort:     smtpPort,
		username:     username,
		password:     password,
		fromAddr:     fromAddr,
		messageChan:  make(chan *Message, 100),
		stopChan:     make(chan struct{}),
		logger:       logger,
		pollInterval: 30 * time.Second, // Check for emails every 30 seconds
	}
}

// Name returns "email".
func (ec *EmailConnector) Name() string {
	return "email"
}

// Start begins polling for email messages (if IMAP configured).
func (ec *EmailConnector) Start(ctx context.Context) error {
	ec.logger.Info("starting Email connector", "smtp_host", ec.smtpHost, "from", ec.fromAddr)

	// Verify SMTP is accessible
	if err := ec.IsHealthy(ctx); !err {
		return fmt.Errorf("SMTP server not accessible")
	}

	// Start polling for incoming emails if IMAP configured
	if ec.imapHost != "" {
		go ec.pollMessages(ctx)
	}

	return nil
}

// Stop gracefully shuts down the Email connector.
func (ec *EmailConnector) Stop(ctx context.Context) error {
	ec.logger.Info("stopping Email connector")
	close(ec.stopChan)
	return nil
}

// Send sends an email message.
func (ec *EmailConnector) Send(ctx context.Context, resp *Response) error {
	// resp.SourceID is expected to be the recipient email address
	toAddr := resp.SourceID
	if !strings.Contains(toAddr, "@") {
		return fmt.Errorf("invalid email address: %s", toAddr)
	}

	// Build email headers and body
	subject := "Gorkbot Response"
	if subj, ok := resp.Metadata["subject"]; ok {
		subject = subj
	}

	message := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		ec.fromAddr, toAddr, subject, resp.Text,
	)

	// Connect to SMTP server
	auth := smtp.PlainAuth("", ec.username, ec.password, ec.smtpHost)
	addr := net.JoinHostPort(ec.smtpHost, fmt.Sprintf("%d", ec.smtpPort))

	err := smtp.SendMail(addr, auth, ec.fromAddr, []string{toAddr}, []byte(message))
	if err != nil {
		ec.logger.Error("failed to send email", "error", err, "to", toAddr)
		return err
	}

	ec.logger.Info("sent email", "to", toAddr)
	return nil
}

// MessageChan returns the channel for incoming messages.
func (ec *EmailConnector) MessageChan() <-chan *Message {
	return ec.messageChan
}

// IsHealthy checks if the SMTP server is accessible.
func (ec *EmailConnector) IsHealthy(ctx context.Context) bool {
	addr := net.JoinHostPort(ec.smtpHost, fmt.Sprintf("%d", ec.smtpPort))

	// Try to dial SMTP with timeout
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		ec.logger.Error("email health check failed", "error", err)
		return false
	}
	defer conn.Close()

	// Try EHLO/HELO
	client, err := smtp.NewClient(conn, ec.smtpHost)
	if err != nil {
		return false
	}
	defer client.Close()

	// Enable TLS if supported
	if ok, _ := client.Extension("STARTTLS"); ok {
		client.StartTLS(&tls.Config{ServerName: ec.smtpHost})
	}

	// Authenticate
	auth := smtp.PlainAuth("", ec.username, ec.password, ec.smtpHost)
	if err := client.Auth(auth); err != nil {
		ec.logger.Error("email auth check failed", "error", err)
		return false
	}

	return true
}

// pollMessages would check IMAP inbox for new emails.
// This is a placeholder for a more complete implementation.
func (ec *EmailConnector) pollMessages(ctx context.Context) {
	ticker := time.NewTicker(ec.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ec.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// IMAP receive path is optional; keep the loop alive and observable
			// until a full inbound adapter is wired.
			ec.logger.Debug("email polling tick (IMAP receive not configured)",
				"imap_host", ec.imapHost,
				"imap_port", ec.imapPort,
			)
		}
	}
}
