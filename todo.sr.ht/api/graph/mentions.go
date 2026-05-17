package graph

import (
	"context"
	"regexp"
	"strconv"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/config"

	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
)

var (
	// ~username
	userMentionRE = regexp.MustCompile(`(^|[\s(]?)?~(\w+)\b([^/]|$)`)
	// ~username/tracker-name#id
	// tracker-name#id
	// #id
	ticketMentionRE = regexp.MustCompile(`(^|[\s(]?)(~(\w+)/)?([A-Za-z0-9_.-]+)?\#(\d+)\b`)
	// https://todo.example.org/~username/trackername/id
	ticketURLRE = regexp.MustCompile(`(^|[\s(]?)(https?://[A-Za-z0-9.]+)/(~(\w+)/)([A-Za-z0-9_.-]+)/(\d+)\b`)
)

// Stores state associated with user or ticket mentions
type Mentions struct {
	Users   map[string]struct{}
	Tickets map[string]model.Ticket // Partially filled in
}

func ScanMentions(ctx context.Context, tracker *model.Tracker,
	ticket *model.Ticket, body string) Mentions {
	user := auth.ForContext(ctx)
	conf := config.ForContext(ctx)
	origin := config.GetOrigin(conf, "todo.sr.ht", true)

	mentionedUsers := make(map[string]struct{})
	mentionedTickets := make(map[string]model.Ticket)
	matches := userMentionRE.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if len(match) != 4 {
			panic("Invalid regex match")
		}
		mentionedUsers[match[2]] = struct{}{}
	}

	matches = ticketMentionRE.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		var (
			username    string
			trackerName string
			ticketID    int
		)
		if len(match) != 6 {
			panic("Invalid regex match")
		}
		if match[3] != "" {
			username = match[3]
		} else {
			username = user.Username
		}
		if match[4] != "" {
			trackerName = match[4]
		} else {
			trackerName = tracker.Name
		}
		ticketID, _ = strconv.Atoi(match[5])
		tik := model.Ticket{
			ID:          ticketID,
			TrackerName: trackerName,
			OwnerName:   username,
		}
		mentionedTickets[tik.Ref()] = tik
	}

	matches = ticketURLRE.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if len(match) != 7 {
			panic("Invalid regex match")
		}
		var (
			root        = match[2]
			username    = match[4]
			trackerName = match[5]
		)
		if root != origin {
			continue
		}
		ticketID, _ := strconv.Atoi(match[6])
		mentionedTickets[ticket.Ref()] = model.Ticket{
			ID:          ticketID,
			TrackerName: trackerName,
			OwnerName:   username,
		}
	}

	return Mentions{
		Users:   mentionedUsers,
		Tickets: mentionedTickets,
	}
}
