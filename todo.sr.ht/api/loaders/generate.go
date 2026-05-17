//go:build generate
// +build generate

package loaders

import (
	_ "github.com/vektah/dataloaden"
)

//go:generate go run github.com/vektah/dataloaden EntitiesByParticipantIDLoader int git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model.Entity
//go:generate ./gen UsersByIDLoader int api/graph/model.User
//go:generate ./gen UsersByNameLoader string api/graph/model.User
//go:generate ./gen TrackersByIDLoader int api/graph/model.Tracker
//go:generate ./gen TrackersByNameLoader string api/graph/model.Tracker
//go:generate ./gen TrackersByOwnerNameLoader [2]string api/graph/model.Tracker
//go:generate ./gen TicketsByIDLoader int api/graph/model.Ticket
//go:generate ./gen TicketsByTrackerIDLoader [2]int api/graph/model.Ticket
//go:generate ./gen CommentsByIDLoader int api/graph/model.Comment
//go:generate ./gen LabelsByIDLoader int api/graph/model.Label
//go:generate ./gen SubsByTicketIDLoader int api/graph/model.TicketSubscription
//go:generate ./gen SubsByTrackerIDLoader int api/graph/model.TrackerSubscription
//go:generate ./gen ParticipantsByUserIDLoader int api/graph/model.Participant
//go:generate ./gen ParticipantsByUsernameLoader string api/graph/model.Participant
