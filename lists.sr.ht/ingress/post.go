// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/email"
	"git.sr.ht/~sircmpwn/core-go/errors"
	apierr "git.sr.ht/~sircmpwn/lists.sr.ht/api/errors"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

func (b *Backend) Post(sender *Sender, data []byte, msg *message.Entity, list *MailingList) error {
	if !(sender.ACL.Post || (list.IsReply && sender.ACL.Reply)) { //nolint:staticcheck
		return &PostPermError{sender, list}
	}

	if err := Validate(sender, msg, list); err != nil {
		return err
	}

	msgID := msg.Header.Get("Message-ID")

	// Validate() consumed the message.Entity body, directly archive raw data
	err := ArchiveMessage(data, list)

	switch {
	case err == nil:
		log.Printf("Archived %s to %s", msgID, list.FullName())
		return b.ForwardMessage(data, list)

	case errors.Is(err, apierr.ErrDuplicateEmail):
		log.Printf("Dropping duplicate message %s on %s", msgID, list.FullName())
		return nil

	default:
		return err
	}
}

func (b *Backend) ForwardMessage(data []byte, list *MailingList) error {
	var recipients []string
	alreadyCopied := make(map[string]bool)

	// fetch subscriber emails from database
	subscribers, err := LookupSubscribers(list)
	if err != nil {
		return err
	}

	// cannot fail, the message has already been validated
	msg, _ := message.Read(bytes.NewReader(data))

	// eliminate recipients that were already included in the original message
	var from string
	senderIsCopied := false
	header := mail.Header{Header: msg.Header}
	for idx, name := range []string{"From", "To", "Cc"} {
		addresses, _ := header.AddressList(name)
		for _, addr := range addresses {
			if idx == 0 {
				from = addr.Address
			} else if addr.Address == from {
				senderIsCopied = true
			}
			alreadyCopied[addr.Address] = true
		}
	}
	for _, email := range subscribers {
		if !alreadyCopied[email] {
			recipients = append(recipients, email)
		}
	}
	if !senderIsCopied && CopySelf(from) {
		recipients = append(recipients, from)
	}

	msgID := msg.Header.Get("Message-Id")

	if len(recipients) == 0 {
		log.Printf("No recipients to forward message %s to.", msgID)
		return nil
	}

	// prepend message with appropriate mailing list headers
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "List-Unsubscribe: <mailto:%s?subject=unsubscribe>\r\n",
		list.PlusAddress(CMD_UNSUBSCRIBE))
	fmt.Fprintf(&buf, "List-Subscribe: <mailto:%s?subject=subscribe>\r\n",
		list.PlusAddress(CMD_SUBSCRIBE))
	fmt.Fprintf(&buf, "List-Archive: <%s/%s>\r\n",
		Config.OriginUrl, list.FullName())
	fmt.Fprintf(&buf, "Archived-At: <%s/%s/%s>\r\n",
		Config.OriginUrl, list.FullName(), url.PathEscape(msgID))
	fmt.Fprintf(&buf, "List-Post: <mailto:%s>\r\n",
		list.Address())
	fmt.Fprintf(&buf, "List-ID: %s <%s.%s>\r\n",
		list.FullName(), list.FullName(), Config.Domain)
	fmt.Fprintf(&buf, "Sender: %s <%s>\r\n",
		list.FullName(), list.Address())

	// append received message verbatim without reformatting
	_, err = buf.Write(data)
	if err != nil {
		return fmt.Errorf("buf.Write: %w", err)
	}

	// forward the message to all subscribers
	log.Printf("Forwarding message %s to %d subscribers",
		msgID, len(recipients))
	ForwardsCounter.Inc()
	return email.EnqueueRaw(b.ctx, buf.Bytes(), recipients)
}

var (
	requiredHeaders   = []string{"From", "Subject", "Message-Id"}
	prohibitedHeaders = []string{"Return-Receipt-To", "Disposition-Notification-To"}
)

func Validate(sender *Sender, msg *message.Entity, list *MailingList) error {
	for _, h := range requiredHeaders {
		if !msg.Header.Has(h) {
			return InvalidHeaderErrorf("The %s header is required.", h)
		}
	}
	for _, h := range prohibitedHeaders {
		if msg.Header.Has(h) {
			return InvalidHeaderErrorf("The %s header is prohibited.", h)
		}
	}
	if !msg.Header.Has("To") && !msg.Header.Has("Cc") {
		return InvalidHeaderErrorf("The To or Cc header is required.")
	}
	foundTextPart := false
	var rejected []string

	err := msg.Walk(func(path []int, part *message.Entity, err error) error {
		if err != nil {
			return err
		}
		contentType, _, err := part.Header.ContentType()
		if err != nil {
			contentType = "text/plain"
		}
		if strings.HasPrefix(contentType, "multipart/") {
			return nil
		}
		disp, _, err := part.Header.ContentDisposition()
		if err != nil {
			disp = "inline"
		}
		if contentType == "text/plain" && disp == "inline" {
			foundTextPart = true
		}
		permit := false
		for _, mime := range list.PermitMimetypes {
			if match, _ := filepath.Match(mime, contentType); match {
				permit = true
				break
			}
		}
		if !permit {
			rejected = append(rejected, contentType)
		}
		for _, mime := range list.RejectMimetypes {
			if match, _ := filepath.Match(mime, contentType); match {
				rejected = append(rejected, contentType)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(rejected) != 0 {
		if rejected[0] == "text/html" && !foundTextPart {
			return &HtmlError{sender}
		} else {
			return &ForbidenMimeError{sender, rejected[0]}
		}
	}
	if !foundTextPart {
		return &NoTextError{sender}
	}
	return nil
}
