package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v81"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/loaders"
)

// Retrieves a payment intent from its ID.
func GetPaymentIntent(
	ctx context.Context,
	sub *model.BillingSubscription,
) (model.PaymentIntent, error) {
	client := ForContext(ctx, sub.Currency)
	intent, err := client.PaymentIntents.Get(*sub.IntentID, &stripe.PaymentIntentParams{
		Expand: []*string{
			stripe.String("payment_method"),
			stripe.String("latest_charge"),
		},
	})
	if err != nil {
		return nil, err
	}

	subID, err := strconv.Atoi(intent.Metadata["subscription_id"])
	if err != nil {
		panic(err)
	}

	var taxRate *float64
	if rateStr, ok := intent.Metadata["tax_rate"]; ok {
		rate, err := strconv.ParseFloat(rateStr, 64)
		if err != nil {
			panic(err)
		}
		taxRate = &rate
	}

	taxDue, err := strconv.Atoi(intent.Metadata["tax_due"])
	if err != nil {
		panic(err)
	}

	var addr model.BillingAddress
	err = json.Unmarshal([]byte(intent.Metadata["billing_address"]), &addr)
	if err != nil {
		panic(err)
	}

	conf := config.ForContext(ctx)
	pubKey, ok := Get(conf, sub.Currency, "stripe-public-key")
	if !ok {
		panic(fmt.Errorf("no stripe public key"))
	}

	var method *model.PaymentMethod
	if intent.PaymentMethod != nil {
		method, err = getMethodFromPaymentIntent(ctx, intent, sub.Currency)
		if err != nil {
			return nil, err
		}
	}

	return &model.StripePaymentIntent{
		ID:             intent.ID,
		TotalDue:       int(intent.Amount),
		TaxRate:        taxRate,
		TaxDue:         taxDue,
		Outcome:        stripeIntentToOutcome(intent),
		Method:         method,
		ClientSecret:   intent.ClientSecret,
		PublicKey:      pubKey,
		SubscriptionID: subID,
	}, nil
}

// Creates an on-session Stripe payment intent (i.e. one the user will pay for
// interactively) for the given billing subscription.
func CreatePaymentIntent(
	ctx context.Context,
	sub *model.BillingSubscription,
	addr *model.BillingAddress,
	idempotencyKey *string,
) (model.PaymentIntent, error) {
	client := ForContext(ctx, sub.Currency)

	_, customer, err := lookupOrCreateCustomer(ctx, sub.Currency, sub.UserID)
	if err != nil {
		return nil, err
	}

	invoice, err := InvoiceFor(ctx, sub, addr)
	if err != nil {
		return nil, err
	}
	params := stripe.PaymentIntentParams{
		Customer:         stripe.String(customer.ID),
		Amount:           stripe.Int64(int64(invoice.Total)),
		Currency:         stripe.String(strings.ToLower(invoice.Currency.String())),
		SetupFutureUsage: stripe.String(string(stripe.PaymentIntentSetupFutureUsageOffSession)),
		Metadata:         genIntentMetadata(sub, addr, invoice),
	}
	if idempotencyKey == nil {
		idempotencyKey = stripe.String(uuid.New().String())
	}
	params.SetIdempotencyKey(*idempotencyKey)

	intent, err := client.PaymentIntents.New(&params)
	if err != nil {
		return nil, err
	}

	conf := config.ForContext(ctx)
	pubKey, ok := Get(conf, sub.Currency, "stripe-public-key")
	if !ok {
		panic(fmt.Errorf("no stripe public key"))
	}

	return &model.StripePaymentIntent{
		ID:             intent.ID,
		IdempotencyKey: *idempotencyKey,
		BillingAddress: addr,
		TotalDue:       invoice.Total,
		TaxRate:        invoice.TaxRate,
		TaxDue:         invoice.TaxCharged,
		ClientSecret:   intent.ClientSecret,
		PublicKey:      pubKey,
		SubscriptionID: sub.ID,
	}, nil
}

// Creates and immediately confirms an off-session (i.e. non-interactive)
// Stripe payment intent to charge the user's subscription renewal fee.
func ChargeRenewalPayment(
	ctx context.Context,
	sub *model.BillingSubscription,
	addr *model.BillingAddress,
	method *model.PaymentMethod,
) (model.PaymentIntent, error) {
	client := ForContext(ctx, sub.Currency)

	user, customer, err := lookupOrCreateCustomer(ctx, sub.Currency, sub.UserID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if user.InternalPaymentDue == nil ||
		user.InternalPaymentDue.After(now) {
		panic(fmt.Errorf("attempted to charge renewal for user without payment due"))
	}
	if sub.Status != model.SubscriptionStatusActive {
		panic(fmt.Errorf("attempted to charge renewal for inactive subscription"))
	}
	if !sub.Autorenew {
		return nil, ErrCancelled
	}

	invoice, err := InvoiceFor(ctx, sub, addr)
	if err != nil {
		return nil, err
	}

	params := stripe.PaymentIntentParams{
		Customer:      stripe.String(customer.ID),
		Amount:        stripe.Int64(int64(invoice.Total)),
		Currency:      stripe.String(strings.ToLower(invoice.Currency.String())),
		OffSession:    stripe.Bool(true),
		Confirm:       stripe.Bool(true),
		PaymentMethod: stripe.String(method.ProcessorID),
		Metadata:      genIntentMetadata(sub, addr, invoice),
	}

	intent, err := client.PaymentIntents.New(&params)
	if err != nil {
		switch err := err.(type) {
		case *stripe.Error:
			if err.PaymentIntent != nil {
				// In this case we want to update the intent
				// variable but disregard the error per-se and
				// move on to returning a FAILED payment
				// outcome.
				intent = err.PaymentIntent
			} else {
				return nil, err
			}
		default:
			return nil, err
		}
	}

	return &model.StripePaymentIntent{
		ID:             intent.ID,
		BillingAddress: addr,
		TotalDue:       invoice.Total,
		TaxRate:        invoice.TaxRate,
		TaxDue:         invoice.TaxCharged,
		SubscriptionID: sub.ID,
		Method:         method,
		Outcome:        stripeIntentToOutcome(intent),
	}, nil
}

func CreateSetupIntent(
	ctx context.Context,
	sub *model.BillingSubscription,
) (model.SetupIntent, error) {
	client := ForContext(ctx, sub.Currency)
	user, customer, err := lookupOrCreateCustomer(
		ctx, sub.Currency, sub.UserID)
	if err != nil {
		return nil, err
	}

	params := stripe.SetupIntentParams{
		Customer: stripe.String(customer.ID),
		Metadata: map[string]string{
			"user_id": strconv.Itoa(user.ID),
		},
	}
	intent, err := client.SetupIntents.New(&params)
	if err != nil {
		return nil, err
	}

	conf := config.ForContext(ctx)
	pubKey, ok := Get(conf, sub.Currency, "stripe-public-key")
	if !ok {
		panic(fmt.Errorf("no stripe public key"))
	}

	return &model.StripeSetupIntent{
		ID:           intent.ID,
		ClientSecret: &intent.ClientSecret,
		PublicKey:    pubKey,
		Status:       model.SetupIntentStatusPending,
	}, nil
}

func GetSetupIntent(
	ctx context.Context,
	sub *model.BillingSubscription,
	intentID string,
) (model.SetupIntent, error) {
	client := ForContext(ctx, sub.Currency)
	intent, err := client.SetupIntents.Get(intentID, &stripe.SetupIntentParams{
		Expand: []*string{
			stripe.String("payment_method"),
			stripe.String("latest_attempt"),
		},
	})
	if err != nil {
		return nil, err
	}

	var method *model.PaymentMethod
	if intent.PaymentMethod != nil {
		method, err = getMethodFromSetupIntent(ctx, intent, sub.Currency)
		if err != nil {
			return nil, err
		}
	}

	var status model.SetupIntentStatus
	switch intent.Status {
	case stripe.SetupIntentStatusSucceeded:
		status = model.SetupIntentStatusSucceeded
	case stripe.SetupIntentStatusProcessing:
		status = model.SetupIntentStatusProcessing
	default:
		status = model.SetupIntentStatusCancelled
	}

	return &model.StripeSetupIntent{
		ID:     intent.ID,
		Method: method,
		Status: status,
	}, nil
}

// Removes a payment method from a customer's account at the payment processor.
func DeletePaymentMethod(
	ctx context.Context,
	method *model.PaymentMethod,
) error {
	client := ForContext(ctx, method.Currency)
	_, err := client.PaymentMethods.Detach(method.ProcessorID,
		&stripe.PaymentMethodDetachParams{})
	return err
}

func lookupOrCreateCustomer(
	ctx context.Context,
	currency model.Currency,
	userID int,
) (*model.User, *stripe.Customer, error) {
	client := ForContext(ctx, currency)

	var (
		c   *stripe.Customer
		err error
	)

	user, err := loaders.ForContext(ctx).UsersByID.Load(userID)
	if err != nil {
		return nil, nil, err
	}

	var customerID *string
	if err := database.WithTx(ctx, &sql.TxOptions{
		Isolation: 0,
		ReadOnly:  true,
	}, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT payment_processor_id
			FROM user_payment_processor
			WHERE user_id = $1 AND currency = $2;
		`, userID, currency.String())
		return row.Scan(&customerID)
	}); err != nil {
		if err != sql.ErrNoRows {
			return nil, nil, err
		}
	}

	if customerID != nil {
		c, err := client.Customers.Get(*customerID,
			&stripe.CustomerParams{})
		return user, c, err
	}

	if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
		params := &stripe.CustomerParams{
			Description: stripe.String(user.CanonicalName()),
			Email:       stripe.String(user.Email),
		}
		params.SetIdempotencyKey(fmt.Sprintf(
			"customer.New(user_id=%d)", user.ID))

		c, err = client.Customers.New(params)
		if err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_payment_processor (
				user_id, currency, payment_processor_id
			) VALUES ($1, $2, $3);
		`, userID, currency.String(), c.ID)

		return err
	}); err != nil {
		return nil, nil, err
	}

	return user, c, nil
}

func genIntentMetadata(
	sub *model.BillingSubscription,
	addr *model.BillingAddress,
	invoice *model.Invoice,
) map[string]string {
	addrJSON, err := json.Marshal(addr)
	if err != nil {
		panic(err)
	}

	// TODO: We should probably create the invoice row before we get here,
	// and store an invoice ID instead of the tax metadata
	metadata := map[string]string{
		"subscription_id": fmt.Sprintf("%d", sub.ID),
		"user_id":         fmt.Sprintf("%d", sub.UserID),
		"billing_address": string(addrJSON),
		"tax_due":         strconv.Itoa(invoice.TaxCharged),
	}
	if invoice.TaxRate != nil {
		metadata["tax_rate"] = fmt.Sprintf("%f", *invoice.TaxRate)
	}
	return metadata
}

func stripeIntentToOutcome(intent *stripe.PaymentIntent) model.PaymentOutcome {
	var outcome model.PaymentOutcome

	switch intent.Status {
	case stripe.PaymentIntentStatusSucceeded:
		outcome.Status = model.PaymentIntentStatusSucceeded
	case stripe.PaymentIntentStatusCanceled:
		outcome.Status = model.PaymentIntentStatusCancelled
	case stripe.PaymentIntentStatusProcessing:
		outcome.Status = model.PaymentIntentStatusProcessing
	case stripe.PaymentIntentStatusRequiresPaymentMethod:
		if intent.LastPaymentError != nil && intent.LastPaymentError.Type == stripe.ErrorTypeCard {
			outcome.Status = model.PaymentIntentStatusFailed
			outcome.Error = stripe.String(intent.LastPaymentError.Msg)
		} else {
			outcome.Status = model.PaymentIntentStatusCancelled
		}
	default:
		log.Printf("Payment intent %s: unexpeceted status '%s'",
			intent.ID, intent.Status)
		outcome.Status = model.PaymentIntentStatusFailed
		outcome.Error = stripe.String("An internal error occured. Please contact support.")
	}

	return outcome
}

func getMethodFromPaymentIntent(
	ctx context.Context,
	intent *stripe.PaymentIntent,
	currency model.Currency,
) (*model.PaymentMethod, error) {
	// iDEAL and Bancontact both indirectly set up a SEPA direct debit and
	// this is ultimately the actual payment method that we need to use for
	// future payments. It has to be extracted from the list of charges.
	client := ForContext(ctx, currency)

	var (
		methodID string
		setupVia string
	)
	switch intent.PaymentMethod.Type {
	case stripe.PaymentMethodTypeIDEAL:
		setupVia = "iDEAL"
		methodID = intent.LatestCharge.PaymentMethodDetails.
			IDEAL.GeneratedSEPADebit.ID
	case stripe.PaymentMethodTypeBancontact:
		setupVia = "Bancontact"
		methodID = intent.LatestCharge.PaymentMethodDetails.
			Bancontact.GeneratedSEPADebit.ID
	default:
		return stripePaymentMethod(intent.PaymentMethod,
			currency, ""), nil
	}

	method, err := client.PaymentMethods.Get(methodID,
		&stripe.PaymentMethodParams{})
	if err != nil {
		return nil, err
	}
	return stripePaymentMethod(method, currency, setupVia), nil
}

func getMethodFromSetupIntent(
	ctx context.Context,
	intent *stripe.SetupIntent,
	currency model.Currency,
) (*model.PaymentMethod, error) {
	// iDEAL and Bancontact both indirectly set up a SEPA direct debit and
	// this is ultimately the actual payment method that we need to use for
	// future payments. It has to be extracted from the list of charges.
	client := ForContext(ctx, currency)

	var (
		methodID string
		setupVia string
	)
	switch intent.PaymentMethod.Type {
	case stripe.PaymentMethodTypeIDEAL:
		setupVia = "iDEAL"
		methodID = intent.LatestAttempt.PaymentMethodDetails.
			IDEAL.GeneratedSEPADebit.ID
	case stripe.PaymentMethodTypeBancontact:
		setupVia = "Bancontact"
		methodID = intent.LatestAttempt.PaymentMethodDetails.
			Bancontact.GeneratedSEPADebit.ID
	default:
		return stripePaymentMethod(intent.PaymentMethod,
			currency, ""), nil
	}

	method, err := client.PaymentMethods.Get(methodID,
		&stripe.PaymentMethodParams{})
	if err != nil {
		return nil, err
	}
	return stripePaymentMethod(method, currency, setupVia), nil
}

func stripePaymentMethod(
	stripeMethod *stripe.PaymentMethod,
	currency model.Currency,
	setupVia string,
) *model.PaymentMethod {
	if stripeMethod == nil {
		return nil
	}

	method := model.PaymentMethod{
		ProcessorID: stripeMethod.ID,
		Created:     time.Unix(stripeMethod.Created, 0),
	}

	switch stripeMethod.Type {
	case stripe.PaymentMethodTypeCard:
		method.Name = fmt.Sprintf("%s ending in %s",
			cardBrandName(stripeMethod.Card.Brand),
			stripeMethod.Card.Last4)

		expires := time.Date(
			int(stripeMethod.Card.ExpYear),
			time.Month(stripeMethod.Card.ExpMonth),
			1, 0, 0, 0, 0, time.UTC)
		method.Expires = &expires
	case stripe.PaymentMethodTypeSEPADebit:
		if setupVia != "" {
			method.Name = fmt.Sprintf("SEPA account ending in %s (via %s)",
				stripeMethod.SEPADebit.Last4,
				setupVia)
		} else {
			method.Name = fmt.Sprintf("SEPA account ending in %s",
				stripeMethod.SEPADebit.Last4)
		}
	default:
		panic(fmt.Errorf("unsupported payment method %s", stripeMethod.Type))
	}

	return &method
}

func cardBrandName(brand stripe.PaymentMethodCardBrand) string {
	switch brand {
	case stripe.PaymentMethodCardBrandAmex:
		return "American Express"
	case stripe.PaymentMethodCardBrandDiners:
		return "Diners Club"
	case stripe.PaymentMethodCardBrandDiscover:
		return "Discover"
	case stripe.PaymentMethodCardBrandJCB:
		return "JCB"
	case stripe.PaymentMethodCardBrandMastercard:
		return "Mastercard"
	case stripe.PaymentMethodCardBrandUnionpay:
		return "Unionpay"
	case stripe.PaymentMethodCardBrandVisa:
		return "Visa"
	default:
		return "Unknown card"
	}
}
