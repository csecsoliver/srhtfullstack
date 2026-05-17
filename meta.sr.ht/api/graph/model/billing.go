package model

import (
	"context"
	"database/sql"
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
	"github.com/shopspring/decimal"
)

type BillingAddress struct {
	FullName     *string `json:"fullName,omitempty"`
	BusinessName *string `json:"businessName,omitempty"`
	Address1     *string `json:"address1,omitempty"`
	Address2     *string `json:"address2,omitempty"`
	City         *string `json:"city,omitempty"`
	Region       *string `json:"region,omitempty"`
	Postcode     *string `json:"postcode,omitempty"`
	// ISO 3166 country code
	Country *string `json:"country,omitempty"`
	// Value-added tax number (EU)
	Vat *string `json:"vat,omitempty"`

	ID int
}

type PaymentOutcome struct {
	Status PaymentIntentStatus `json:"status"`
	Error  *string             `json:"error,omitempty"`
}

// Returns true if this payment outcome is in a state where the user's payment
// was drawn from their account
func (outcome PaymentOutcome) Charged() bool {
	return outcome.Status == PaymentIntentStatusSucceeded ||
		outcome.Status == PaymentIntentStatusPending
}

type PaymentMethod struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	Created  time.Time  `json:"created"`
	Expires  *time.Time `json:"expires"`
	Currency Currency   `json:"currency"`

	ProcessorID string
}

type BillingSubscription struct {
	ID       int                `json:"id"`
	Created  time.Time          `json:"created"`
	Updated  time.Time          `json:"updated"`
	Status   SubscriptionStatus `json:"status"`
	Currency Currency           `json:"currency"`
	Interval PaymentInterval    `json:"interval"`
	Payment  PaymentOutcome     `json:"payment"`
	// If true, payment is automatically renewed when term ellapses.
	Autorenew bool `json:"autorenew"`

	UserID    int
	ProductID int
	IntentID  *string
}

func (sub *BillingSubscription) Price(ctx context.Context) (*ProductPrice, error) {
	var price ProductPrice

	if err := database.WithTx(ctx, &sql.TxOptions{
		ReadOnly:  true,
		Isolation: 0,
	}, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT amount, currency
			FROM product_price
			WHERE product_id = $1 AND currency = $2;
		`, sub.ProductID, sub.Currency.String())

		return row.Scan(
			&price.Amount,
			&price.Currency,
		)
	}); err != nil {
		return nil, err
	}

	return &price, nil
}

func (sub *BillingSubscription) Subtotal(ctx context.Context) (int, error) {
	productPrice, err := sub.Price(ctx)
	if err != nil {
		return 0, err
	}

	amount := decimal.NewFromInt(int64(productPrice.Amount))

	if sub.Interval == PaymentIntervalAnnually {
		// Apply annual discount (2 months free)
		amount = amount.Mul(decimal.NewFromInt(10))
	}

	return int(amount.IntPart()), nil
}

// A paid product available for purchase.
type Product struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Retired    bool   `json:"retired"`
	Subsidized bool   `json:"subsidized"`
}

// Price point for a product in a given currency.
type ProductPrice struct {
	Amount   int      `json:"amount"`
	Currency Currency `json:"currency"`
}

func (p *Product) Prices(ctx context.Context) ([]ProductPrice, error) {
	var prices []ProductPrice

	if err := database.WithTx(ctx, &sql.TxOptions{
		Isolation: 0,
		ReadOnly:  true,
	}, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT amount, currency
			FROM product_price
			WHERE product_id = $1
		`, p.ID)
		if err != nil {
			return err
		}

		for rows.Next() {
			var price ProductPrice
			err = rows.Scan(
				&price.Amount,
				&price.Currency,
			)
			if err != nil {
				return err
			}
			prices = append(prices, price)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return prices, nil
}

type PaymentIntent interface {
	IsPaymentIntent()
	GetID() string
	GetSubscriptionID() int
	GetOutcome() PaymentOutcome
	GetMethod() *PaymentMethod
}

type StripePaymentIntent struct {
	ID             string          `json:"id"`
	BillingAddress *BillingAddress `json:"billingAddress"`
	TotalDue       int             `json:"totalDue"`
	TaxRate        *float64        `json:"taxRate,omitempty"`
	TaxDue         int             `json:"taxDue"`
	Outcome        PaymentOutcome  `json:"outcome"`
	IdempotencyKey string          `json:"idempotencyKey"`
	ClientSecret   string          `json:"clientSecret"`
	Method         *PaymentMethod  `json:"method"`
	PublicKey      string          `json:"publicKey"`

	SubscriptionID int
}

func (StripePaymentIntent) IsPaymentIntent() {}

func (intent *StripePaymentIntent) GetID() string {
	return intent.ID
}

func (intent *StripePaymentIntent) GetSubscriptionID() int {
	return intent.SubscriptionID
}

func (intent *StripePaymentIntent) GetOutcome() PaymentOutcome {
	return intent.Outcome
}

func (intent *StripePaymentIntent) GetMethod() *PaymentMethod {
	return intent.Method
}

type SetupIntent interface {
	IsSetupIntent()
	GetMethod() *PaymentMethod
	GetStatus() SetupIntentStatus
}

type StripeSetupIntent struct {
	ID           string            `json:"id"`
	Method       *PaymentMethod    `json:"method,omitempty"`
	Status       SetupIntentStatus `json:"status"`
	ClientSecret *string           `json:"clientSecret,omitempty"`
	PublicKey    string            `json:"publicKey"`
}

func (StripeSetupIntent) IsSetupIntent() {}

func (intent *StripeSetupIntent) GetMethod() *PaymentMethod {
	return intent.Method
}

func (intent *StripeSetupIntent) GetStatus() SetupIntentStatus {
	return intent.Status
}
