package model

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type Invoice struct {
	ID           int       `json:"id"`
	Issued       time.Time `json:"issued"`
	ServiceStart time.Time `json:"serviceStart"`
	ServiceEnd   time.Time `json:"serviceEnd"`
	Currency     Currency  `json:"currency"`
	Total        int       `json:"total"`

	InternalInvoiceNo int

	UUID      uuid.UUID
	ProductID int
	UserID    int

	// More details for generating invoices (not usually populated)
	Subtotal int
	// Note: this field should not be used for monetary arithmetic (convert
	// to decimal first)
	TaxRate    *float64
	TaxCharged int
	ReverseVAT bool
	Interval   PaymentInterval

	alias  string
	fields *database.ModelFields
}

func (inv *Invoice) InvoiceNo() string {
	return fmt.Sprintf("%03d-%004d", inv.UserID, inv.InternalInvoiceNo)
}

func (inv *Invoice) FormatPrice() string {
	switch inv.Currency {
	case CurrencyEUR:
		return fmt.Sprintf("€%.2f", float64(inv.Total)/100)
	case CurrencyUSD:
		return fmt.Sprintf("$%.2f", float64(inv.Total)/100)
	}
	panic(fmt.Errorf("invalid currency"))
}

func (inv *Invoice) As(alias string) *Invoice {
	inv.alias = alias
	return inv
}

func (i *Invoice) Alias() string {
	return i.alias
}

func (i *Invoice) Table() string {
	return "invoice"
}

func (i *Invoice) Product(ctx context.Context) (*Product, error) {
	var product Product

	if err := database.WithTx(ctx, &sql.TxOptions{
		Isolation: 0,
		ReadOnly:  true,
	}, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT id, name, retired, subsidized
			FROM product
			WHERE id = $1;
		`, i.ProductID)

		return row.Scan(
			&product.ID,
			&product.Name,
			&product.Retired,
			&product.Subsidized,
		)
	}); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &product, nil
}

func (i *Invoice) Fields() *database.ModelFields {
	if i.fields != nil {
		return i.fields
	}
	i.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "invoice_no", GQL: "", Ptr: &i.InternalInvoiceNo},
			{SQL: "issued", GQL: "issued", Ptr: &i.Issued},
			{SQL: "service_start", GQL: "serviceStart", Ptr: &i.ServiceStart},
			{SQL: "service_end", GQL: "serviceEnd", Ptr: &i.ServiceEnd},
			{SQL: "currency", GQL: "currency", Ptr: &i.Currency},
			{SQL: "total", GQL: "total", Ptr: &i.Total},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &i.ID},
			{SQL: "uuid", GQL: "", Ptr: &i.UUID},
			{SQL: "user_id", GQL: "", Ptr: &i.UserID},
			{SQL: "product_id", GQL: "", Ptr: &i.ProductID},
		},
	}
	return i.fields
}

func (inv *Invoice) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*Invoice, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(inv.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(inv.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var invoices []*Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(database.Scan(ctx, &inv)...); err != nil {
			panic(err)
		}
		invoices = append(invoices, &inv)
	}

	if len(invoices) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(invoices[len(invoices)-1].ID),
			Search: cur.Search,
		}
		invoices = invoices[:cur.Count]
	} else {
		cur = nil
	}

	return invoices, cur
}
