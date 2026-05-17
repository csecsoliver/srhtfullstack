// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"fmt"
)

type BounceError interface {
	Error() string
	Body() string
}

type HtmlError struct{ sender *Sender }

func (e *HtmlError) Error() string { return "text/html part found" }
func (e *HtmlError) Body() string {
	return fmt.Sprintf(`Hi %s!

We received your email, but were unable to deliver it because it
contains HTML, which has been disabled by the administrators of the
mailing list you are trying to reach. HTML emails are not permitted on
this list.

This guide can help you configure your client to send in plain text
instead:

%s

This is an automated email. You may reply to this email to reach the
administrators of the mailing list software if you think this response
was sent in error. Note that we are NOT responsible for the policy of
this specific mailing list, we are NOT affiliated with this specific
mailing list or the project that uses it, and we can NOT help you with
your original inquiry.
`, e.sender.Name, Config.RejectUrl)
}

type ForbidenMimeError struct {
	sender *Sender
	mime   string
}

func (e *ForbidenMimeError) Error() string {
	return fmt.Sprintf("forbidden MIME part: %s", e.mime)
}

func (e *ForbidenMimeError) Body() string {
	return fmt.Sprintf(`Hi %s!

We received your email, but were unable to deliver it because it 
contains content which has been blacklisted by the list admin. Please 
remove your %s attachments and send again. 

You are also advised to configure your email client to send emails in 
plain text to avoid additional errors in the future:

%s

This is an automated email. You may reply to this email to reach the
administrators of the mailing list software if you think this response
was sent in error. Note that we are NOT responsible for the policy of
this specific mailing list, we are NOT affiliated with this specific
mailing list or the project that uses it, and we can NOT help you with
your original inquiry.
`, e.sender.Name, e.mime, Config.RejectUrl)
}

type NoTextError struct{ sender *Sender }

func (e *NoTextError) Error() string { return "no text/plain part found" }
func (e *NoTextError) Body() string {
	return fmt.Sprintf(`Hi %s!

We received your email, but were unable to deliver it because there were 
no text/plain parts. Our mail system requires all emails to have at 
least one plain text part. The following guide can help you configure 
your client to send in plain text:

%s

This is an automated email. You may reply to this email to reach the
administrators of the mailing list software if you think this response
was sent in error. Note that we are NOT responsible for the policy of
this specific mailing list, we are NOT affiliated with this specific
mailing list or the project that uses it, and we can NOT help you with
your original inquiry.
`, e.sender.Name, Config.RejectUrl)
}

type UnknownListError struct{ list string }

func (e *UnknownListError) Error() string {
	return fmt.Sprintf("unknown list: %s", e.list)
}

func (e *UnknownListError) Body() string {
	return fmt.Sprintf(`Hi!

We received your email, but were unable to deliver it because the 
mailing list you wrote to was not found:

%s

The correct posting addresses are:

~username/list-name@%s

Or if your mail system has trouble sending to addresses with ~ or / in 
them, you can use:

u.username.list-name@%s

If your mail system does not support our normal posting addresses, we 
would appreciate it if you wrote to your mail admin to ask them to fix 
their system. Our posting addresses are valid per RFC-5322.

If you have any questions, please reply to this email to reach the mail 
admin. We apologise for the inconvenience.
`, e.list, Config.Domain, Config.Domain)
}

type UnknownCommandError struct{ list *MailingList }

func (e *UnknownCommandError) Error() string {
	return fmt.Sprintf("unknown list command: +%s", e.list.Command)
}

func (e *UnknownCommandError) Body() string {
	return fmt.Sprintf(`Hi!

We received your email, but were unable to process it because the 
destination address has an invalid +%s suffix command.

The supported addresses related to this mailing list are:

%s
	To post messages on the list
%s
	To request to subscribe
%s
	To confirm a subscription request
%s
	To request to unsubscribe
%s
	To confirm an unusubscription request

If you have any questions, please reply to this email to reach the mail 
admin. We apologise for the inconvenience.
`,
		e.list.Command,
		e.list.Address(),
		e.list.PlusAddress(CMD_SUBSCRIBE),
		e.list.PlusAddress(CMD_CONFIRM_SUB),
		e.list.PlusAddress(CMD_UNSUBSCRIBE),
		e.list.PlusAddress(CMD_CONFIRM_UNSUB))
}

type PostPermError struct {
	sender *Sender
	list   *MailingList
}

func (e *PostPermError) Error() string {
	return fmt.Sprintf("%s denied posting to %s",
		e.sender.Email, e.list.Address())
}

func (e *PostPermError) Body() string {
	return fmt.Sprintf(`Hi %s!

Sorry, but your account is not allowed to post to: %s
`, e.sender.Name, e.list.Address())
}

type SubscribePermError struct {
	sender *Sender
	list   *MailingList
}

func (e *SubscribePermError) Error() string {
	return fmt.Sprintf("%s denied subscribing to %s",
		e.sender.Email, e.list.Address())
}

func (e *SubscribePermError) Body() string {
	s := fmt.Sprintf(`Hi %s!

We got your request to subscribe to: %s

but unfortunately subscriptions to this list are restricted. 
Your request has been disregarded.
`, e.sender.Name, e.list.FullName())
	if e.sender.ACL.Post {
		s += fmt.Sprintf(`
However, you are permitted to post mail to this list at this address:

%s
`, e.list.Address())
	}
	return s
}

type InvalidHeaderError struct{ message string }

func (e *InvalidHeaderError) Error() string { return e.message }
func (e *InvalidHeaderError) Body() string  { return e.message }

func InvalidHeaderErrorf(format string, v ...any) error {
	return &InvalidHeaderError{fmt.Sprintf(format, v...)}
}

type ConfirmationError struct {
	sender      *Sender
	list        *MailingList
	subject     string
	unsubscribe bool
}

func (e *ConfirmationError) Error() string {
	return fmt.Sprintf("confirmation error: command=+%s subject=%s",
		e.list.Command, e.subject)
}

func (e *ConfirmationError) Body() string {
	kind := "a subscription"
	if e.unsubscribe {
		kind = "an unsubscription"
	}
	return fmt.Sprintf(`Hi %s!

We received what looked like %s request confirmation 
email for the %s mailing list.

Unfortunately, we could not find any pending requests matching the 
message subject.

Subject: %s

You should have received a confirmation email from us that instructed 
to reply to the message, keeping the subject intact.

If you need more help, please reply to this email to reach the mail 
admin. We apologise for the inconvenience.
`, kind, e.sender.Name, e.list.Address(), e.subject)
}
