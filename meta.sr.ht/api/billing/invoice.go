package billing

import (
	_ "embed"

	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"git.sr.ht/~sircmpwn/core-go/database"
	country "github.com/mikekonan/go-countries"
	"github.com/vaughan0/go-ini"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/loaders"
)

var (
	//go:embed invoice.typ
	invoiceTemplateIn string
	invoiceTemplate   *template.Template
	isProduction      bool
)

func invoiceInit(conf ini.File) {
	if !IsEnabled(conf) {
		return
	}

	if env, ok := conf.Get("sr.ht", "environment"); ok {
		isProduction = env == "production"
	}

	invoiceTemplate = template.Must(
		template.New("invoice").
			Funcs(map[string]any{
				"escape": func(s string) string {
					s = strings.ReplaceAll(s, "\\", "\\\\")
					s = strings.ReplaceAll(s, "\"", "\\\"")
					return s
				},
				"countryName": func(code string) string {
					// TBH if the great firewall blocks us
					// it would probably save us a lot of
					// trouble with DDoS traffic
					if code == "TW" {
						return "Taiwan"
					}

					c, _ := country.ByAlpha2CodeStr(code)
					name := c.NameStr()
					// This strings.ReplaceAll courtesy of ISO 3166
					if strings.HasSuffix(name, " (the)") {
						name = strings.ReplaceAll(name, " (the)", "")
					}
					return name
				},
				"formatPrice": func(currency model.Currency, amt int) string {
					switch currency {
					case model.CurrencyEUR:
						return fmt.Sprintf("€%.2f", float64(amt)/100)
					case model.CurrencyUSD:
						return fmt.Sprintf("$%.2f", float64(amt)/100)
					}
					panic(fmt.Errorf("invalid currency"))
				},
				"formatPercent": func(amt *float64) string {
					return fmt.Sprintf("%.1f%%", *amt*100)
				},
			}).
			Parse(invoiceTemplateIn))

	letterhead := invoiceTemplate.New("letterhead")
	if path, ok := conf.Get("meta.sr.ht::billing", "letterhead-template"); ok {
		file, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			panic(err)
		}

		template.Must(letterhead.Parse(string(data)))
	} else {
		template.Must(letterhead.Parse(""))
	}
}

type TemplateContext struct {
	Invoice   *model.Invoice
	User      *model.User
	Address   *model.BillingAddress
	Products  []TemplateProduct
	Discounts []TemplateDiscount
	Sample    bool
}

type TemplateProduct struct {
	Product   *model.Product
	UnitPrice *model.ProductPrice
	Units     int
	Total     int
}

type TemplateDiscount struct {
	Reason string
	Amount int
}

// Generates a PDF from a model.Invoice.
func GenerateInvoice(
	ctx context.Context,
	w io.Writer,
	invoice *model.Invoice,
) error {
	var (
		err       error
		source    bytes.Buffer
		pdf       []byte
		user      *model.User
		addr      *model.BillingAddress
		products  []TemplateProduct
		discounts []TemplateDiscount
	)

	user, err = loaders.ForContext(ctx).UsersByID.Load(invoice.UserID)
	if err != nil {
		return err
	}

	if err := database.WithTx(ctx, &sql.TxOptions{
		ReadOnly:  true,
		Isolation: 0,
	}, func(tx *sql.Tx) error {
		var address model.BillingAddress

		row := tx.QueryRowContext(ctx, `
		SELECT
			full_name,
			business_name,
			address_1,
			address_2,
			city,
			region,
			postcode,
			country,
			vat
		FROM "user"
		JOIN billing_address
			ON billing_address_id = billing_address.id
		WHERE "user".id = $1
		`, user.ID)

		err := row.Scan(
			&address.FullName,
			&address.BusinessName,
			&address.Address1,
			&address.Address2,
			&address.City,
			&address.Region,
			&address.Postcode,
			&address.Country,
			&address.Vat)
		if err != nil {
			return err
		}
		addr = &address
		return nil
	}); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		// Leave everything NULL in case no billing address exists for
		// this user. In the future we may require all users to define
		// a billing address with at least their country of residence,
		// but for now it's fine without.
	}

	product, err := invoice.Product(ctx)
	if err != nil {
		return err
	}
	prices, err := product.Prices(ctx)
	if err != nil {
		return err
	}

	var price *model.ProductPrice
	for _, p := range prices {
		if p.Currency != invoice.Currency {
			continue
		}

		var units int
		switch invoice.Interval {
		case model.PaymentIntervalMonthly:
			units = 1
		case model.PaymentIntervalAnnually:
			units = 12
		default:
			panic(fmt.Errorf("invalid payment interval"))
		}

		products = append(products, TemplateProduct{
			Product:   product,
			UnitPrice: &p,
			Units:     units,
			Total:     p.Amount * units,
		})

		price = &p
		break
	}

	if invoice.Interval == model.PaymentIntervalAnnually {
		discounts = append(discounts, TemplateDiscount{
			Reason: "Annual payment discount",
			Amount: price.Amount * 2,
		})
	}

	err = invoiceTemplate.Execute(&source, &TemplateContext{
		Invoice:   invoice,
		User:      user,
		Address:   addr,
		Products:  products,
		Discounts: discounts,
		Sample:    !isProduction,
	})
	if err != nil {
		return err
	}

	cmd := exec.Command("typst", "compile", "-f", "pdf", "-", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error redirecting stdin: %s", err)
	}

	go func() {
		defer stdin.Close()
		stdin.Write(source.Bytes())
	}()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error redirecting stderr: %s", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error redirecting stdout: %s", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error running typst: %s", err)
	}

	errlog, err := io.ReadAll(stderr)
	if err != nil {
		return fmt.Errorf("error reading stderr: %s", err)
	}

	pdf, err = io.ReadAll(stdout)
	if err != nil {
		return fmt.Errorf("error reading stdout: %s", err)
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("typst failed: %s", string(errlog))
		return err
	}

	_, err = w.Write(pdf)
	return err
}
