package errors

import (
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func newGqlError(message, code string) *gqlerror.Error {
	return &gqlerror.Error{
		Message:    message,
		Extensions: map[string]any{"code": code},
	}
}

var (
	ErrPaymentRequired = newGqlError(
		"A paid account is required to use this feature.",
		"ERR_PAYMENT_REQUIRED")
)
