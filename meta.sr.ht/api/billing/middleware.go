package billing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stripe/stripe-go/v81/client"
	"github.com/vaughan0/go-ini"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

type contextKey struct {
	name string
}

var ctxKey = &contextKey{"billing"}

func Middleware(conf ini.File) func(next http.Handler) http.Handler {
	if !IsEnabled(conf) {
		panic(fmt.Errorf("attempted to create billing middleware with billing disabled"))
	}

	clients := make(map[model.Currency]*client.API)

	for _, c := range model.AllCurrency {
		skey, ok := Get(conf, c, "stripe-secret-key")
		if !ok {
			panic(fmt.Errorf("[meta.sr.ht::billing]stripe-secret-key missing"))
		}
		clients[c] = client.New(skey, nil)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKey, clients)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// Returns the Stripe client associated with a currency on this context.
func ForContext(ctx context.Context, currency model.Currency) *client.API {
	raw, ok := ctx.Value(ctxKey).(map[model.Currency]*client.API)
	if !ok {
		panic(fmt.Errorf("invalid billing context"))
	}
	return raw[currency]
}
