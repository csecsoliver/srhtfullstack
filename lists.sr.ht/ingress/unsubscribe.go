// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/email"
	"git.sr.ht/~sircmpwn/core-go/errors"
	apierr "git.sr.ht/~sircmpwn/lists.sr.ht/api/errors"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

func (b *Backend) Unsubscribe(sender *Sender, msg *message.Entity, list *MailingList) error {
	if !sender.ACL.Browse {
		return &SubscribePermError{sender, list}
	}

	log.Printf("Unsubscribing %s from %s", sender.Email, list.FullName())
	token, err := RequestUnsubscription(sender, list)

	switch {
	case err == nil:
		return b.SendUnsubscribeConfirmation(sender, msg, list, token)
	case errors.Is(err, apierr.ErrNotSubscribed):
		return b.NotSubscribed(sender, msg, list)
	default:
		return err
	}
}

func (b *Backend) SendUnsubscribeConfirmation(
	sender *Sender, msg *message.Entity, list *MailingList, token string,
) error {
	from := list.PlusAddress(CMD_CONFIRM_UNSUB)
	header := ReplyHeaders(from, "confirm "+token, msg)
	header.Set("Reply-To", from)

	var body bytes.Buffer

	fmt.Fprintf(&body, `Hi %s!

We have received a request for unsubscription of your email address, 
%s, from the following mailing list:

%s

To confirm that you want to be removed from this mailing list, simply 
reply to this message, keeping the Subject: header intact.

Note that simply sending a reply to this message should work from 
most mail readers, since that usually leaves the Subject: line in the 
right form (additional "Re:" text in the Subject: is okay).

If you do not wish to be unsubscribed from this list, please simply 
disregard this message. If you think you are being maliciously 
unsubscribed from the list, or have any other questions, please reach 
out to %s.
`, sender.Name, sender.Email, list.Address(), Config.OwnerAddr)

	return email.EnqueueStd(b.ctx, header, &body, nil)
}

func (b *Backend) NotSubscribed(sender *Sender, msg *message.Entity, list *MailingList) error {
	var header mail.Header
	subject := SubjectFallback(msg, "Your unsubscription request")
	ReplyHeaders(Config.MailerAddr, "Re: "+subject, msg)
	header.Set("Reply-To", Config.OwnerAddr)

	var body bytes.Buffer

	fmt.Fprintf(&body, `Hi %s!

We got an email asking to unsubscribe you from the %s mailing list.

However, it looks like you are not subscribed. To subscribe, send an 
email to:

%s

Feel free to reply to this email if you have any questions.
`, sender.Name, list.FullName(), list.PlusAddress(CMD_SUBSCRIBE))

	return email.EnqueueStd(b.ctx, header, &body, nil)
}

func (b *Backend) ConfirmUnsubscribe(
	sender *Sender, msg *message.Entity, list *MailingList,
) error {
	subject, _ := msg.Header.Text("Subject")
	tokens := strings.Fields(subject)
	if len(tokens) == 0 {
		return &ConfirmationError{sender, list, subject, true}
	}
	token := tokens[len(tokens)-1]

	log.Printf("Confirming unsubscription from %s for %s",
		list.FullName(), sender.Email)
	err := ConfirmUnsubscription(sender, list, token)
	switch {
	case err == nil:
		break
	case errors.Is(err, apierr.ErrInvalidToken):
	case errors.Is(err, apierr.ErrSubscriptionNotFound):
		return &ConfirmationError{sender, list, subject, true}
	default:
		return err
	}

	header := ReplyHeaders(Config.MailerAddr, subject, msg)
	header.Set("Reply-To", Config.OwnerAddr)

	var body bytes.Buffer

	fmt.Fprintf(&body, `Hi %s!

You have been successfully unsubscribed from the %s mailing list. 
If you wish to re-subscribe, send an email to:

%s

Feel free to reply to this email if you have any questions.
`, sender.Name, list.FullName(), list.PlusAddress(CMD_SUBSCRIBE))

	return email.EnqueueStd(b.ctx, header, &body, nil)
}
