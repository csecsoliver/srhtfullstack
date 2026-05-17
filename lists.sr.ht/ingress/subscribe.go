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

func (b *Backend) Subscribe(sender *Sender, msg *message.Entity, list *MailingList) error {
	if !sender.ACL.Browse {
		return &SubscribePermError{sender, list}
	}

	log.Printf("Subscribing %s to %s", sender.Email, list.FullName())
	token, err := RequestSubscription(sender, list)

	switch {
	case err == nil:
		return b.SendSubscribeConfirmation(sender, msg, list, token)
	case errors.Is(err, apierr.ErrAlreadySubscribed):
		return b.AlreadySubscribed(sender, msg, list)
	default:
		return err
	}
}

func (b *Backend) SendSubscribeConfirmation(
	sender *Sender, msg *message.Entity, list *MailingList, token string,
) error {
	from := list.PlusAddress(CMD_CONFIRM_SUB)
	header := ReplyHeaders(from, "confirm "+token, msg)
	header.Set("Reply-To", from)

	var body bytes.Buffer

	fmt.Fprintf(&body, `Hi %s!

We have received a request for subscription of your email address, 
%s, to the following mailing list:

%s

To confirm that you want to be added to this mailing list, simply reply 
to this message, keeping the Subject: header intact.

Note that simply sending a reply to this message should work from 
most mail readers, since that usually leaves the Subject: line in the 
right form (additional "Re:" text in the Subject: is okay).

If you do not wish to be subscribed to this list, please simply 
disregard this message. If you think you are being maliciously 
subscribed to the list, or have any other questions, please reach out to
%s.
`, sender.Name, sender.Email, list.Address(), Config.OwnerAddr)

	return email.EnqueueStd(b.ctx, header, &body, nil)
}

func (b *Backend) AlreadySubscribed(sender *Sender, msg *message.Entity, list *MailingList) error {
	var header mail.Header
	subject := SubjectFallback(msg, "Your subscription request")
	ReplyHeaders(Config.MailerAddr, "Re: "+subject, msg)
	header.Set("Reply-To", Config.OwnerAddr)

	var body bytes.Buffer

	fmt.Fprintf(&body, `Hi %s!

We got an email asking to subscribe you to the %s mailing list.

However, it looks like you're already subscribed. To unsubscribe, 
send an email to:

%s

Feel free to reply to this email if you have any questions.
`, sender.Name, list.FullName(), list.PlusAddress(CMD_UNSUBSCRIBE))

	return email.EnqueueStd(b.ctx, header, &body, nil)
}

func (b *Backend) ConfirmSubscribe(
	sender *Sender, msg *message.Entity, list *MailingList,
) error {
	subject, _ := msg.Header.Text("Subject")
	tokens := strings.Fields(subject)
	if len(tokens) == 0 {
		return &ConfirmationError{sender, list, subject, false}
	}
	token := tokens[len(tokens)-1]

	log.Printf("Confirming subscription to %s for %s",
		list.FullName(), sender.Email)
	err := ConfirmSubscription(sender, list, token)
	switch {
	case err == nil:
		break
	case errors.Is(err, apierr.ErrInvalidToken):
		return &ConfirmationError{sender, list, subject, false}
	default:
		return err
	}

	header := ReplyHeaders(Config.MailerAddr, subject, msg)
	header.Set("Reply-To", Config.OwnerAddr)

	var body bytes.Buffer

	fmt.Fprintf(&body, `Hi %s!

Your subscription to %s is confirmed!

To post to the list, send an email to:

%s

To unsubscribe in the future, send an email to this address:

%s

Feel free to reply to this email if you have any general questions for the 
admins of the mailing list platform (not the list itself).
`, sender.Name, list.Address(), list.Address(), list.PlusAddress(CMD_UNSUBSCRIBE))

	return email.EnqueueStd(b.ctx, header, &body, nil)
}
