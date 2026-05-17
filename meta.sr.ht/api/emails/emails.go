package emails

import (
	"context"
	"fmt"
	"log"
	"strings"
	"text/template"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/email"
	"github.com/emersion/go-message/mail"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

func SendRegistrationConfirmation(ctx context.Context,
	user *model.User, pgpKey *string, token string) {
	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Confirm your %s registration", siteName))

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Username  string
		Origin    string
		Token     string
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Username:  user.Username,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		Token:     token,
	}

	tmpl := template.Must(template.New("security-event").Parse(`Hello ~{{.Username}}!

You (or someone pretending to be you) have registered for an account on
{{.SiteName}}. 

To complete your registration, please follow this link:

{{.Origin}}/confirm-account/{{.Token}}

If not, just ignore this email. If you have any questions, please reply
to this email.

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	log.Printf("Send registration email to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendEmailNotification(ctx context.Context,
	username, userEmail, message string, pgpKey *string) error {
	r := strings.NewReader(message)
	mr, err := mail.CreateReader(r)
	if err != nil {
		return err
	}
	defer mr.Close()
	// We expect exactly one plain text part
	p, err := mr.NextPart()
	if err != nil {
		return err
	}
	if _, ok := p.Header.(*mail.InlineHeader); !ok {
		return fmt.Errorf("sending attachments is not supported")
	}
	_, err = mr.NextPart()
	if err == nil {
		return fmt.Errorf("sending multi-part mails is not supported")
	}

	header := mr.Header
	// Assert that caller does not try to set any recipients
	for _, h := range []string{"To", "Cc", "Bcc"} {
		rcpts, err := header.AddressList(h)
		if err != nil {
			return err
		}
		if len(rcpts) > 0 {
			return fmt.Errorf("%s header must not be set", h)
		}
	}
	// and that we at least have a subject
	if subject, err := header.Subject(); err != nil || subject == "" {
		return fmt.Errorf("missing or malformed subject")
	}

	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: username, Address: userEmail},
	})

	log.Printf("Send notification email to %s", userEmail)
	return email.EnqueueStd(ctx, header, p.Body, pgpKey)
}

// Send a security-related notice to the given user.
// Always prefer using `sendSecurityNotification` if possible.
func SendSecurityNotificationTo(ctx context.Context,
	username, address, subject, details string, pgpKey *string) {
	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: username, Address: address},
	})
	header.SetSubject(subject)

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Username  string
		Details   string
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Username:  username,
		Details:   details,
	}

	tmpl := template.Must(template.New("security-event").Parse(`~{{.Username}},

This email was sent to inform you that the following security-sensitive
event has occured on your {{.SiteName}} account:

{{.Details}}

If you did not expect this to occur, please reply to this email urgently
to contact support. Otherwise, no action is required.

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendEmailChangeConfirmation(ctx context.Context, user *model.User,
	pgpKey *string, newEmail, token string) {
	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var (
		h1 mail.Header
		h2 mail.Header
	)

	h1.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	h2.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: newEmail},
	})

	h1.SetSubject(fmt.Sprintf("Your email address on %s is changing", siteName))
	h2.SetSubject(fmt.Sprintf("Confirm your new %s email address", siteName))

	type TemplateContext struct {
		Token     string
		NewEmail  string
		OwnerName string
		Origin    string
		SiteName  string
		Username  string
	}
	tctx := TemplateContext{
		Token:     token,
		NewEmail:  newEmail,
		OwnerName: ownerName,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		SiteName:  siteName,
		Username:  user.Username,
	}

	m1tmpl := template.Must(template.New("update_email_old").Parse(`Hi ~{{.Username}}!

This is a notice that your email address on {{.SiteName}} is being
changed to {{.NewEmail}}. A confirmation email is being sent to
{{.NewEmail}} to finalize the process.

If you did not expect this to happen, please reply to this email
urgently to reach support.

-- 
{{.OwnerName}}
{{.SiteName}}`))

	m2tmpl := template.Must(template.New("update_email_new").Parse(`Hi ~{{.Username}}!

You (or someone pretending to be you) updated the email address for
your account to {{.NewEmail}}. To confirm the new email and apply the
change, click the following link:

{{.Origin}}/change-email/{{.Token}}

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var (
		m1body strings.Builder
		m2body strings.Builder
	)
	err := m1tmpl.Execute(&m1body, tctx)
	if err != nil {
		panic(err)
	}

	err = m2tmpl.Execute(&m2body, tctx)
	if err != nil {
		panic(err)
	}

	err = email.EnqueueStd(ctx, h1, strings.NewReader(m1body.String()), pgpKey)
	if err != nil {
		panic(err)
	}

	err = email.EnqueueStd(ctx, h2, strings.NewReader(m2body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendPasswordResetConfirmation(ctx context.Context, user *model.User,
	pgpKey *string, token string) {
	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Reset your %s password", siteName))

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Username  string
		Origin    string
		Token     string
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Username:  user.Username,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		Token:     token,
	}

	tmpl := template.Must(template.New("security-event").Parse(`Hello ~{{.Username}}!

You (or someone pretending to be you) has requested a password reset for your
account on {{.SiteName}}. If you wish to reset your password, click this link:

{{.Origin}}/reset-password/{{.Token}}

If you weren't expecting this, just ignore it. Your account is safe, and this
link will expire in 48 hours.

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	log.Printf("Send password change email to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendPaymentSuccessNotification(ctx context.Context,
	user *model.User, pgpKey *string,
	sub *model.BillingSubscription,
	invoice *model.Invoice) {

	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Your %s payment was processed successfully", siteName))

	var interval string
	switch sub.Interval {
	case model.PaymentIntervalMonthly:
		interval = "monthly"
	case model.PaymentIntervalAnnually:
		interval = "annual"
	}

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Origin    string
		User      *model.User
		Sub       *model.BillingSubscription
		Invoice   *model.Invoice
		Interval  string
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		User:      user,
		Sub:       sub,
		Invoice:   invoice,
		Interval:  interval,
	}

	tmpl := template.Must(template.New("payment-success").Parse(`Hello {{.User.CanonicalName}}!

We have successfully processed your {{.Interval}} payment of {{.Invoice.FormatPrice}}.

You can view your invoice for this and other payments online:

{{.Origin}}/billing/invoices

If you have any questions or feedback, please reply to this email.

Thank you for supporting {{.SiteName}}!

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	// TODO: Attach the generated invoice PDF to this email
	log.Printf("Send successful payment notification to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendPaymentFailureNotification(ctx context.Context,
	user *model.User, pgpKey *string,
	sub *model.BillingSubscription,
	outcome model.PaymentOutcome) {

	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Action required: your %s payment could not be processed", siteName))

	var interval string
	switch sub.Interval {
	case model.PaymentIntervalMonthly:
		interval = "monthly"
	case model.PaymentIntervalAnnually:
		interval = "annual"
	}

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Origin    string
		User      *model.User
		Sub       *model.BillingSubscription
		Outcome   model.PaymentOutcome
		Interval  string
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		User:      user,
		Sub:       sub,
		Outcome:   outcome,
		Interval:  interval,
	}

	tmpl := template.Must(template.New("payment-failed").Parse(`Hello {{.User.CanonicalName}}!

We were unable to charge your {{.Interval}} payment:

{{.Outcome.Error}}

Your account is now past due, and your access to paid services may be affected.
This payment will be automatically retried over the next few days.

To review your options, please visit the {{.SiteName}} billing dashboard:

{{.Origin}}/billing

To reach support directly, you may reply to this email.

Thank you for supporting {{.SiteName}}!

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	log.Printf("Send failed payment notice to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendSettlementFailureNotification(ctx context.Context,
	user *model.User, pgpKey *string,
	sub *model.BillingSubscription,
	outcome model.PaymentOutcome) {

	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Action required: your %s payment could not be processed", siteName))

	var interval string
	switch sub.Interval {
	case model.PaymentIntervalMonthly:
		interval = "monthly"
	case model.PaymentIntervalAnnually:
		interval = "annual"
	}

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Origin    string
		User      *model.User
		Sub       *model.BillingSubscription
		Outcome   model.PaymentOutcome
		Interval  string
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		User:      user,
		Sub:       sub,
		Outcome:   outcome,
		Interval:  interval,
	}

	tmpl := template.Must(template.New("payment-failed").Parse(`Hello {{.User.CanonicalName}}!

We were unable to charge your {{.Interval}} payment:

{{.Outcome.Error}}

Because this was the first payment of your subscription, your
subscription attempt has been cancelled and your account has been
converted to a non-paying account.

To retry, please visit the {{.SiteName}} billing dashboard:

{{.Origin}}/billing

To reach support directly, you may reply to this email.

Thank you for supporting {{.SiteName}}!

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	log.Printf("Send failed settlement notice to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendPaymentCancellationNotification(ctx context.Context,
	user *model.User, pgpKey *string,
	sub *model.BillingSubscription) {

	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Your %s services have been cancelled", siteName))

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Origin    string
		User      *model.User
		Sub       *model.BillingSubscription
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		User:      user,
		Sub:       sub,
	}

	tmpl := template.Must(template.New("payment-failed").Parse(`Hello {{.User.CanonicalName}}!

Your paid {{.SiteName}} subscription was cancelled on {{.Sub.Updated.Format "January 2, 2006"}}.
This email is a follow-up on your service cancellation request to inform you
that your subscription term is completed and your cancellation was processed
successfully. You will no longer receive paid services on your account and no
further payments will be processed.

If you wish to renew, please visit the {{.SiteName}} billing dashboard:

{{.Origin}}/billing

To reach support directly, you may reply to this email.

Thank you for supporting {{.SiteName}}!

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	log.Printf("Send payment cancellation notice to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}

func SendSubsidyEndedNotification(ctx context.Context,
	user *model.User, pgpKey *string) {

	conf := config.ForContext(ctx)
	siteName, ok := conf.Get("sr.ht", "site-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]site-name in config"))
	}
	ownerName, ok := conf.Get("sr.ht", "owner-name")
	if !ok {
		panic(fmt.Errorf("expected [sr.ht]owner-name in config"))
	}

	var header mail.Header
	header.SetAddressList("To", []*mail.Address{
		&mail.Address{Name: "~" + user.Username, Address: user.Email},
	})
	header.SetSubject(fmt.Sprintf("Your %s financial aid term has expired", siteName))

	type TemplateContext struct {
		OwnerName string
		SiteName  string
		Origin    string
		User      *model.User
	}
	tctx := TemplateContext{
		OwnerName: ownerName,
		SiteName:  siteName,
		Origin:    config.GetOrigin(conf, "meta.sr.ht", true),
		User:      user,
	}

	tmpl := template.Must(template.New("payment-failed").Parse(`Hello {{.User.CanonicalName}}!

You previously requested financial aid for the use of {{.SiteName}} services.
At this time, your financial aid term has expired. Paid services are no longer
available on your account.

If your situation has changed and you would like to sign up for paid services
now, please visit the {{.SiteName}} billing dashboard:

{{.Origin}}/billing

If your situation is unchanged and you wish to renew your application for
financial aid, please reply to this email.

-- 
{{.OwnerName}}
{{.SiteName}}`))

	var body strings.Builder
	err := tmpl.Execute(&body, tctx)
	if err != nil {
		panic(err)
	}

	log.Printf("Send end of subsidy notice to %s", user.Email)
	err = email.EnqueueStd(ctx, header,
		strings.NewReader(body.String()), pgpKey)
	if err != nil {
		panic(err)
	}
}
