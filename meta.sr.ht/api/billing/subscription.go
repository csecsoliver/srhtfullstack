package billing

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/emails"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

var (
	ErrCancelled = fmt.Errorf("billing subscription cancelled")
)

// Calculates the payment details for a given billing subscription and returns
// an incomplete invoice record with all of the monetary values set accordingly
// (e.g. tax rate), to facilitate calculating the amount to charge the user.
func InvoiceFor(
	ctx context.Context,
	sub *model.BillingSubscription,
	addr *model.BillingAddress,
) (*model.Invoice, error) {
	subtotal, err := sub.Subtotal(ctx)
	if err != nil {
		return nil, err
	}

	inv := &model.Invoice{
		Currency: sub.Currency,
		Subtotal: subtotal,
	}

	cfg := config.ForContext(ctx)
	ApplyTax(cfg, inv, addr)

	return inv, nil
}

// Validate a subscription after a payment intent has been processed to update
// the subscription based on the payment outcome, issuing an invoice/receipt
// and setting the user payment status appropriately, etc.
func ValidateSubscription(
	ctx context.Context,
	tx *sql.Tx,
	user *model.User,
	sub *model.BillingSubscription,
	intent model.PaymentIntent,
) error {
	outcome := intent.GetOutcome()

	// Ensure idempotency
	if *sub.IntentID != intent.GetID() {
		return nil
	}

	switch outcome.Status {
	case model.PaymentIntentStatusSucceeded:
		sub.Status = model.SubscriptionStatusActive
		sub.IntentID = nil
	case model.PaymentIntentStatusProcessing:
		sub.Status = model.SubscriptionStatusSettlement
	default:
		return processFailedPayment(ctx, tx, user, sub, outcome)
	}

	var (
		addr   model.BillingAddress
		addrID *int
	)
	row := tx.QueryRowContext(ctx, `
		SELECT
			billing_address.id,
			full_name, business_name, address_1, address_2,
			city, region, postcode, country, vat
		FROM "user"
		JOIN billing_address
			ON billing_address_id = billing_address.id
		WHERE "user".id = $1;
	`, user.ID)

	err := row.Scan(
		&addr.ID,
		&addr.FullName,
		&addr.BusinessName,
		&addr.Address1,
		&addr.Address2,
		&addr.City,
		&addr.Region,
		&addr.Postcode,
		&addr.Country,
		&addr.Vat,
	)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error fetching billing address: %w", err)
	} else if err != sql.ErrNoRows {
		addrID = &addr.ID
	}

	// Update subscription
	_, err = tx.ExecContext(ctx, `
		UPDATE subscription
		SET
			status = $2,
			updated = NOW() at time zone 'utc',
			payment_outcome = $3,
			payment_intent = $4
		WHERE id = $1;
	`, sub.ID, sub.Status, outcome.Status, sub.IntentID)
	if err != nil {
		return fmt.Errorf("error updating subscription: %w", err)
	}

	// Store payment method
	method := intent.GetMethod()
	row = tx.QueryRowContext(ctx, `
		INSERT INTO payment_method (
			user_id,
			currency,
			created,
			expires,
			name,
			processor_id
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
		ON CONFLICT ON CONSTRAINT payment_method_processor_id_key
		DO UPDATE SET name = $5
		RETURNING id;`,
		user.ID,
		sub.Currency,
		method.Created,
		method.Expires,
		method.Name,
		method.ProcessorID)

	err = row.Scan(&method.ID)
	if err != nil {
		return fmt.Errorf("error storing payment method: %w", err)
	}

	// Prepare an invoice
	invoice, err := InvoiceFor(ctx, sub, &addr)
	if err != nil {
		return fmt.Errorf("error preparing invoice: %w", err)
	}

	invoice.UUID = uuid.New()
	invoice.UserID = user.ID
	invoice.ProductID = sub.ProductID
	invoice.Interval = sub.Interval
	invoice.Issued = time.Now().UTC()
	invoice.ServiceStart = time.Now().UTC()
	switch sub.Interval {
	case model.PaymentIntervalMonthly:
		invoice.ServiceEnd = invoice.ServiceStart.AddDate(0, 1, 0)
	case model.PaymentIntervalAnnually:
		invoice.ServiceEnd = invoice.ServiceStart.AddDate(1, 0, 0)
	}

	// Update user
	_, err = tx.ExecContext(ctx, `
		UPDATE "user"
		SET
			payment_status = 'CURRENT',
			payment_due = $2,
			updated = now() at time zone 'utc',
			default_payment_method_id = $3
		WHERE id = $1;
	`, sub.UserID, invoice.ServiceEnd, method.ID)
	if err != nil {
		return fmt.Errorf("error updating user record: %w", err)
	}

	// Store invoice in S3/database
	if outcome.Status == model.PaymentIntentStatusProcessing {
		// Skip if the payment hasn't settled yet
		return nil
	}

	row = tx.QueryRowContext(ctx, `
		UPDATE "user"
		SET next_invoice_no = next_invoice_no + 1
		WHERE id = $1
		RETURNING next_invoice_no - 1;
	`, sub.UserID)
	err = row.Scan(&invoice.InternalInvoiceNo)
	if err != nil {
		return fmt.Errorf("error fetching next invoice no: %w", err)
	}

	err = UploadInvoice(ctx, invoice)
	if err != nil {
		return fmt.Errorf("error uploading invoice: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO invoice (
			uuid, invoice_no, issued,
			billing_address_id, user_id, product_id,
			interval, service_start, service_end,
			currency, subtotal, tax_rate, tax_charged, reverse_vat,
			total,
			payment_id
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9,
			$10, $11, $12, $13, $14,
			$15,
			$16
		)`,
		invoice.UUID.String(),
		invoice.InternalInvoiceNo,
		invoice.Issued,
		addrID,
		sub.UserID,
		sub.ProductID,
		sub.Interval.String(),
		invoice.ServiceStart,
		invoice.ServiceEnd,
		invoice.Currency.String(),
		invoice.Subtotal,
		invoice.TaxRate,
		invoice.TaxCharged,
		invoice.ReverseVAT,
		invoice.Total,
		intent.GetID())
	if err != nil {
		return fmt.Errorf("error storing invoice row: %w", err)
	}

	pgpKey, err := emails.PGPKeyForUser(ctx, user)
	if err != nil {
		return fmt.Errorf("error fetching user PGP key: %w", err)
	}

	emails.SendPaymentSuccessNotification(ctx,
		user, pgpKey, sub, invoice)
	return nil
}

// Update a subscription following a failed payment (e.g. card declined),
// setting the user's account to delinquent and filling in the error details
// for posterity.
func processFailedPayment(
	ctx context.Context,
	tx *sql.Tx,
	user *model.User,
	sub *model.BillingSubscription,
	outcome model.PaymentOutcome,
) error {
	if outcome.Charged() {
		panic(fmt.Errorf("processFailedPayment called for succesful payment"))
	}

	if sub.Status == model.SubscriptionStatusSettlement {
		// In this case reset the subscription to pending so the user
		// can try again from the top
		_, err := tx.ExecContext(ctx, `
		UPDATE subscription
		SET
			payment_outcome = $2,
			payment_error = $3,
			updated = now() at time zone 'utc',
			status = 'PENDING'
		WHERE id = $1;
		`, sub.ID, outcome.Status, outcome.Error)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
		UPDATE "user"
		SET payment_status = 'UNPAID'
		WHERE id = $1;
		`, sub.UserID)
		if err != nil {
			return err
		}

		pgpKey, err := emails.PGPKeyForUser(ctx, user)
		if err != nil {
			return err
		}
		emails.SendSettlementFailureNotification(ctx,
			user, pgpKey, sub, outcome)
		return nil
	}

	_, err := tx.ExecContext(ctx, `
	UPDATE subscription
	SET
		payment_outcome = $2,
		payment_error = $3,
		updated = now() at time zone 'utc'
	WHERE id = $1;
	`, sub.ID, outcome.Status, outcome.Error)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
	UPDATE "user"
	SET payment_status = 'DELINQUENT'
	WHERE id = $1;
	`, sub.UserID)
	if err != nil {
		return err
	}

	pgpKey, err := emails.PGPKeyForUser(ctx, user)
	if err != nil {
		return err
	}
	emails.SendPaymentFailureNotification(ctx,
		user, pgpKey, sub, outcome)

	return nil
}

// Finalizes a cancelled subscription whose service term is completed by
// setting the user to a non-paying account.
//
// The subscription may be nil to convert a subsidized user to non-paying.
func FinalizeCancellation(
	ctx context.Context,
	tx *sql.Tx,
	user *model.User,
	sub *model.BillingSubscription,
) error {
	pgpKey, err := emails.PGPKeyForUser(ctx, user)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
	UPDATE "user"
	SET
		payment_status = 'UNPAID',
		payment_due = NULL,
		updated = now() at time zone 'utc'
	WHERE id = $1;
	`, user.ID)

	if err != nil {
		return err
	}

	switch user.InternalPaymentStatus {
	case model.PaymentStatusCurrent, model.PaymentStatusDelinquent:
		if sub == nil || sub.Autorenew {
			panic(fmt.Errorf("FinalizeCancellation: invalid user state"))
		}

		_, err := tx.ExecContext(ctx, `
		UPDATE subscription
		SET
			status = 'INACTIVE',
			updated = now() at time zone 'utc'
		WHERE id = $1;
		`, sub.ID)

		if err != nil {
			return err
		}

		emails.SendPaymentCancellationNotification(ctx, user, pgpKey, sub)
	case model.PaymentStatusSubsidized:
		emails.SendSubsidyEndedNotification(ctx, user, pgpKey)
	default:
		panic(fmt.Errorf("FinalizeCancellation: invalid user state"))
	}

	return nil
}
