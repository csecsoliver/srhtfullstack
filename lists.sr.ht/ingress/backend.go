// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/email"
	"github.com/emersion/go-message"
	"github.com/emersion/go-smtp"
)

// The Backend implements SMTP server methods.
type Backend struct {
	ctx context.Context
}

// NewSession is called after client greeting (EHLO, HELO).
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	if addr, ok := c.Conn().RemoteAddr().(*net.TCPAddr); ok {
		if !config.IsInternalIP(addr.IP) {
			RejectedCounter.Inc()
			return nil, fmt.Errorf("peer not in internal-ipnet")
		}
	}
	s := &Session{remote: c.Conn().RemoteAddr(), backend: b}
	log.Printf("connection accepted: %s", s.remote)
	return s, nil
}

// A Session is returned after successful login.
type Session struct {
	remote  net.Addr
	from    string
	to      []string
	backend *Backend
}

// Discard currently processed message.
func (s *Session) Reset() {
	s.from = ""
	s.to = nil
}

// Free all resources associated with session.
func (s *Session) Logout() error {
	return nil
}

// Authenticate the user using SASL PLAIN.
func (s *Session) AuthPlain(username, password string) error {
	return nil
}

// Set return path for currently processed message.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	return nil
}

// Add recipient for currently processed message.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

// Read currently processed message contents.
//
// r must be consumed before Data returns.
func (s *Session) Data(r io.Reader) error {
	var (
		email *message.Entity
		from  string
		to    []string
		err   error
		buf   bytes.Buffer
		data  []byte
		n     int64
	)

	EmailsCounter.Inc()

	n, err = io.CopyN(&buf, r, Config.MaxMessageSize)
	switch {
	case n == Config.MaxMessageSize:
		err = errors.New("message too big")
		// drain whatever is left in the pipe
		_, _ = io.Copy(io.Discard, r)
		goto end
	case errors.Is(err, io.EOF):
		// message was smaller than max size
		break
	case err != nil:
		goto end
	}

	data = buf.Bytes()

	email, err = message.Read(bytes.NewReader(data))
	if err != nil && !message.IsUnknownCharset(err) {
		// We may get a non-fatal UnknownCharsetError, but that does
		// not mean we should drop the message. Only drop it if parsing
		// completely failed.
		goto end
	}

	switch strings.ToLower(email.Header.Get("Auto-Submitted")) {
	case "auto-generated", "auto-replied":
		// disregard automatic emails like OOO replies
		log.Printf(
			"ignoring automatic message from=%s to=%s message_id=%s subject=%s",
			s.from,
			strings.Join(s.to, ","),
			email.Header.Get("Message-Id"),
			email.Header.Get("Subject"),
		)
		DroppedCounter.Inc()
		goto end
	}

	log.Printf(
		"message received from=%s to=%s message_id=%s subject=%s",
		s.from,
		strings.Join(s.to, ","),
		email.Header.Get("Message-Id"),
		email.Header.Get("Subject"),
	)

	// Make local copies of the values before to ensure the references will
	// still be valid when the queued task function is evaluated.
	from = s.from
	to = s.to
	err = s.backend.ProcessMessage(from, to, data)
	if err != nil {
		// Consider any error during archival to be non-fatal.
		// Return a 421 error code to ask postfix to retry later.
		err = &smtp.SMTPError{
			Code:         421,
			EnhancedCode: smtp.EnhancedCode{4, 0, 0},
			Message:      err.Error(),
		}
	}

end:
	s.to = nil
	s.from = ""
	if err != nil {
		ErrorsCounter.Inc()
	}
	return err
}

func (b *Backend) ProcessMessage(
	from string, recipients []string, data []byte,
) error {
	var (
		email  *message.Entity
		list   *MailingList
		sender *Sender
		err    error
	)

	// cannot fail, we already parsed it once in Session.Data()
	email, _ = message.Read(bytes.NewReader(data))

	for _, to := range recipients {
		sender, list, err = LookupEmailDetails(email, to)
		if err != nil {
			goto next
		}
		switch list.Command {
		case CMD_SUBSCRIBE:
			CommandsCounter.Inc()
			err = b.Subscribe(sender, email, list)
		case CMD_UNSUBSCRIBE:
			CommandsCounter.Inc()
			err = b.Unsubscribe(sender, email, list)
		case CMD_CONFIRM_SUB:
			CommandsCounter.Inc()
			err = b.ConfirmSubscribe(sender, email, list)
		case CMD_CONFIRM_UNSUB:
			CommandsCounter.Inc()
			err = b.ConfirmUnsubscribe(sender, email, list)
		default:
			err = b.Post(sender, data, email, list)
		}
	next:
		if bnc, ok := err.(BounceError); ok {
			b.Bounce(email, from, bnc)
		} else if err != nil {
			log.Printf("ProcessMessage: %s", err)
			return err
		}
	}
	return nil
}

// Instead of letting postfix send an unfriendly bounce message, for some errors
// we send our own bounce message which is a little easier to understand.
func (b *Backend) Bounce(msg *message.Entity, to string, bnc BounceError) {
	if list := msg.Header.Get("List-Id"); list != "" {
		// Don't create backscatter if we're getting emails forwarded
		// from another mailing list.
		log.Printf("would have bounced message %s from mailing list %s: %s",
			msg.Header.Get("Message-Id"), bnc, list)
		return
	}

	subject := SubjectFallback(msg, "Your recent email to "+Config.Domain)
	header := ReplyHeaders(Config.MailerAddr, "Re: "+subject, msg)
	header.Set("Reply-To", Config.OwnerAddr)

	log.Printf("bouncing message %s: %s", msg.Header.Get("Message-Id"), bnc)
	BounceCounter.Inc()

	body := strings.TrimSpace(bnc.Body())
	err := email.EnqueueStd(b.ctx, header, strings.NewReader(body), nil)
	if err != nil {
		log.Printf("failed to write bounce message: %s", err)
	}
}
