//go:build generate
// +build generate

package loaders

import (
	_ "github.com/vektah/dataloaden"
)

//go:generate ./gen UsersByIDLoader int api/graph/model.User
//go:generate ./gen UsersByNameLoader string api/graph/model.User
//go:generate ./gen UsersByEmailLoader string api/graph/model.User
//go:generate ./gen OAuthClientsByIDLoader int api/graph/model.OAuthClient
//go:generate ./gen OAuthClientsByUUIDLoader string api/graph/model.OAuthClient
//go:generate ./gen SubscriptionsByIDLoader int api/graph/model.BillingSubscription
//go:generate ./gen SubscriptionsByUserIDLoader int api/graph/model.BillingSubscription
//go:generate ./gen SubscriptionsByIntentLoader string api/graph/model.BillingSubscription
