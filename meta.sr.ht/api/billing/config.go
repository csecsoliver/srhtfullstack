package billing

import (
	"context"
	"fmt"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/errors"
	"github.com/mikekonan/go-countries"
	"github.com/vaughan0/go-ini"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

// Retrieves an item from the billing configuration. Uses a currency-specific
// config option if relevant, or falls back to the general billing
// configuration if not.
func Get(conf ini.File, currency model.Currency, key string) (string, bool) {
	cursec := fmt.Sprintf("meta.sr.ht::billing::%s", currency.String())
	gensec := "meta.sr.ht::billing"

	if val, ok := conf.Get(cursec, key); ok {
		return val, true
	}
	return conf.Get(gensec, key)
}

// Returns true if billing is enabled in this configuration.
func IsEnabled(conf ini.File) bool {
	enabled, ok := conf.Get("meta.sr.ht::billing", "enabled")
	return ok && enabled == "yes"
}

// Returns an error if billing is not enabled in this configuration.
func RequireEnabled(ctx context.Context) error {
	if !IsEnabled(config.ForContext(ctx)) {
		return errors.New(errors.Unsupported,
			"Billing is not enabled on this instance")
	}
	return nil
}

// Returns true if the given country code is not allowed to receive paid
// services.
func IsProhibitedCountry(ctx context.Context, c country.Country) bool {
	conf := config.ForContext(ctx)
	prohibitions, ok := conf.Get("meta.sr.ht::billing", "prohibited-countries")
	if !ok {
		return false
	}

	for _, code := range strings.Split(prohibitions, " ") {
		if code == c.Alpha2CodeStr() {
			return true
		}
	}

	return false
}
