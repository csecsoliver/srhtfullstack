package billing

import (
	"github.com/shopspring/decimal"
	"github.com/vaughan0/go-ini"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

var TaxRateCountry = map[string]decimal.Decimal{
	// VAT rates per country, EU
	// https://europa.eu/youreurope/business/taxation/vat/vat-rules-rates/index_en.htm
	"AT": decimal.RequireFromString("20"),
	"BE": decimal.RequireFromString("21"),
	"BG": decimal.RequireFromString("20"),
	"CY": decimal.RequireFromString("19"),
	"CZ": decimal.RequireFromString("21"),
	"DE": decimal.RequireFromString("19"),
	"DK": decimal.RequireFromString("25"),
	"EE": decimal.RequireFromString("22"),
	"EL": decimal.RequireFromString("24"),
	"ES": decimal.RequireFromString("21"),
	"FI": decimal.RequireFromString("22.5"),
	"FR": decimal.RequireFromString("20"),
	"HR": decimal.RequireFromString("25"),
	"HU": decimal.RequireFromString("27"),
	"IE": decimal.RequireFromString("23"),
	"IT": decimal.RequireFromString("22"),
	"LT": decimal.RequireFromString("21"),
	"LU": decimal.RequireFromString("17"),
	"LV": decimal.RequireFromString("21"),
	"MT": decimal.RequireFromString("18"),
	"NL": decimal.RequireFromString("21"),
	"PL": decimal.RequireFromString("23"),
	"PT": decimal.RequireFromString("23"),
	"RO": decimal.RequireFromString("19"),
	"SE": decimal.RequireFromString("25"),
	"SI": decimal.RequireFromString("22"),
	"SK": decimal.RequireFromString("23"),
}

func ApplyTax(conf ini.File, inv *model.Invoice, addr *model.BillingAddress) {
	serviceCountry, _ := conf.Get("meta.sr.ht::billing", "service-country")
	subtotal := decimal.NewFromInt(int64(inv.Subtotal)) // cents
	total := decimal.NewFromInt(int64(inv.Subtotal))    // cents

	// TODO: Calculate tax for USD users as well once we're processing US
	// invoices through the Dutch entity
	if addr == nil || inv.Currency != model.CurrencyEUR {
		inv.Total = int(total.Round(0).IntPart())
	} else if *addr.Country != serviceCountry && addr.Vat != nil {
		// Reverse VAT applies to business customers _in_ the EU and
		// _out_ of the service provider's country
		inv.Total = int(total.Round(0).IntPart())
		inv.ReverseVAT = true
	} else if taxRate, ok := TaxRateCountry[*addr.Country]; ok {
		taxRate := taxRate.Div(decimal.NewFromInt(100)) // 0..1
		taxCharged := subtotal.Mul(taxRate)             // cents
		total = total.Add(taxCharged)                   // cents

		taxRateF := taxRate.InexactFloat64()
		inv.TaxRate = &taxRateF
		inv.TaxCharged = int(taxCharged.Round(0).IntPart())
	}

	inv.Total = int(total.Round(0).IntPart())
}
