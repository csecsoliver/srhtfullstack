//go:build generate
// +build generate

package loaders

import (
	_ "github.com/vektah/dataloaden"
)

//go:generate ./gen ACLsByIDLoader int api/graph/model.MailingListACL
//go:generate ./gen EmailsByIDLoader int api/graph/model.Email
//go:generate ./gen EmailsByMessageIDLoader string api/graph/model.Email
//go:generate ./gen MailingListsByIDLoader int api/graph/model.MailingList
//go:generate ./gen MailingListsByNameLoader string api/graph/model.MailingList
//go:generate ./gen MailingListsByOwnerNameLoader [2]string api/graph/model.MailingList
//go:generate ./gen PatchsetsByIDLoader int api/graph/model.Patchset
//go:generate go run github.com/vektah/dataloaden SubscriptionsByIDLoader int git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model.ActivitySubscription
//go:generate ./gen ThreadsByIDLoader int api/graph/model.Thread
//go:generate ./gen UsersByIDLoader int api/graph/model.User
//go:generate ./gen UsersByNameLoader string api/graph/model.User
