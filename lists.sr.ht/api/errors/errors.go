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
	ErrNotSubscribed = newGqlError(
		"Not subscribed to this mailing list.",
		"ERR_NOT_SUBSCRIBED")
	ErrAlreadySubscribed = newGqlError(
		"Already subscribed to this mailing list.",
		"ERR_ALREADY_SUBSCRIBED")
	ErrSubscriptionNotFound = newGqlError(
		"Subscription not found.",
		"ERR_SUBSCRIPTION_NOT_FOUND")
	ErrInvalidToken = newGqlError(
		"Invalid subscription token.",
		"ERR_INVALID_TOKEN")
	ErrDuplicateEmail = newGqlError(
		"Message already archived in this list.",
		"ERR_DUPLICATE_EMAIL")
)
