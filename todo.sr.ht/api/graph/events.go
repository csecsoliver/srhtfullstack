package graph

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"text/template"

	"git.sr.ht/~sircmpwn/core-go/client"
	"git.sr.ht/~sircmpwn/core-go/config"
	"github.com/emersion/go-message/mail"

	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/loaders"
)

type NewTicketDetails struct {
	Body      *string
	Root      string
	TicketURL string
}

var newTicketTemplate = template.Must(template.New("new-ticket").Parse(
	`{{.Body}}

-- 
View on the web: {{.Root}}{{.TicketURL}}`))

type TicketStatusDetails struct {
	Root       string
	TicketURL  string
	EventID    int
	Status     string
	Resolution string
}

var ticketStatusTemplate = template.Must(template.New("ticket-status").Parse(
	`{{if eq .Status "RESOLVED"}}Ticket resolved: {{.Resolution}}{{end}}
-- 
View on the web: {{.Root}}{{.TicketURL}}#event-{{.EventID}}`))

type SubmitCommentDetails struct {
	Comment       string
	Root          string
	TicketURL     string
	EventID       int
	Status        string
	Resolution    string
	StatusUpdated bool
}

var submitCommentTemplate = template.Must(template.New("ticket-comment").Parse(`
{{- if .StatusUpdated -}}
{{- if eq .Status "RESOLVED" -}}
Ticket resolved: {{.Resolution}}

{{else -}}
Ticket re-opened: {{.Status}}

{{end}}{{end -}}
{{.Comment }}

-- 
View on the web: {{.Root}}{{.TicketURL}}#event-{{.EventID}}`))

type TicketAssignedDetails struct {
	Root      string
	TicketURL string
	EventID   int
	Assigned  bool
	Assigner  string
	Assignee  string
}

var ticketAssignedTemplate = template.Must(template.New("ticket-assigned").Parse(`
{{- if .Assigned -}}
~{{.Assigner}} assigned this ticket to ~{{.Assignee}}
{{- else -}}
~{{.Assigner}} unassigned ~{{.Assignee}} from this ticket
{{- end}}

-- 
View on the web: {{.Root}}{{.TicketURL}}#event-{{.EventID}}`))

type EventBuilder struct {
	ctx         context.Context
	tx          *sql.Tx
	eventType   uint
	submitterID int
	tracker     *model.Tracker
	ticket      *model.Ticket
	inReplyTo   *string

	mentionedParticipants []int
	mentions              *Mentions
}

// Creates a new event builder for a given submitter (a participant ID) and a
// certain event type, using the provided context and transaction. This is used
// to provide a common implementation for creating events and notifications,
// building up the list of implicated participants over time.
//
// The order in which you call the subsequent functions is important:
// 1. WithTicket
// 2. AddMentions
// 3. InsertSubscriptions
// 4. InsertNotifications
func NewEventBuilder(ctx context.Context, tx *sql.Tx,
	submitterID int, eventType uint) *EventBuilder {
	// Create a temporary table of all participants affected by this
	// submission. This includes everyone who will be notified about it.
	_, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE event_participant
		ON COMMIT DROP
		AS (SELECT
			-- The affected participant:
			$1::INTEGER AS participant_id,
			-- Events they should be notified of:
			$2::INTEGER AS event_type,
			-- Should they be subscribed to this ticket?
			true AS subscribe
		);
	`, submitterID, eventType)
	if err != nil {
		panic(err)
	}

	return &EventBuilder{
		ctx:         ctx,
		tx:          tx,
		eventType:   eventType,
		submitterID: submitterID,
	}
}

// Associates this event with a ticket and implicates the submitter for the
// appropriate events and notifications.
func (builder *EventBuilder) WithTicket(
	tracker *model.Tracker, ticket *model.Ticket) *EventBuilder {
	builder.tracker = tracker
	builder.ticket = ticket
	_, err := builder.tx.ExecContext(builder.ctx, `
		INSERT INTO event_participant (
			participant_id, event_type, subscribe
		)
		SELECT sub.participant_id, $1, false
		FROM ticket_subscription sub
		WHERE sub.tracker_id = $2 OR sub.ticket_id = $3
	`, builder.eventType, tracker.ID, ticket.PKID)
	if err != nil {
		panic(err)
	}
	return builder
}

// Adds mentions to this event builder
func (builder *EventBuilder) AddMentions(mentions *Mentions) *EventBuilder {
	builder.mentions = mentions
	for user := range mentions.Users {
		part, err := loaders.ForContext(builder.ctx).ParticipantsByUsername.Load(user)
		if err != nil {
			panic(err)
		}
		if part == nil {
			continue
		}
		_, err = builder.tx.ExecContext(builder.ctx, `
			INSERT INTO event_participant (
				participant_id, event_type, subscribe
			) VALUES (
				$1, $2, true
			);
		`, part.ID, model.EVENT_USER_MENTIONED)
		if err != nil {
			panic(err)
		}
		builder.mentionedParticipants = append(builder.mentionedParticipants, part.ID)
	}
	return builder
}

// Creates subscriptions for all affected users
func (builder *EventBuilder) InsertSubscriptions() *EventBuilder {
	_, err := builder.tx.ExecContext(builder.ctx, `
		INSERT INTO ticket_subscription (
			created, updated, ticket_id, participant_id
		)
		SELECT
			NOW() at time zone 'utc',
			NOW() at time zone 'utc',
			$1, participant_id
		FROM event_participant
		WHERE subscribe = true
		ON CONFLICT ON CONSTRAINT subscription_ticket_participant_uq
		DO NOTHING;
	`, builder.ticket.PKID)
	if err != nil {
		panic(err)
	}
	return builder
}

// Adds event_notification records for all affected users and inserts
// ancillary events (such as mentions) and their notifications.
func (builder *EventBuilder) InsertNotifications(eventID int, commentID *int) *EventBuilder {
	_, err := builder.tx.ExecContext(builder.ctx, `
		INSERT INTO event_notification (created, event_id, user_id)
		SELECT
			NOW() at time zone 'utc',
			$1, part.user_id
		FROM event_participant ev
		JOIN participant part ON part.id = ev.participant_id
		WHERE part.user_id IS NOT NULL AND ev.event_type = $2;
	`, eventID, builder.eventType)
	if err != nil {
		panic(err)
	}

	if builder.mentions == nil {
		return builder
	}

	for _, id := range builder.mentionedParticipants {
		var eventID int
		row := builder.tx.QueryRowContext(builder.ctx, `
			INSERT INTO event (
				created, event_type, participant_id, by_participant_id,
				ticket_id, from_ticket_id, comment_id
			) VALUES (
				NOW() at time zone 'utc',
				$1, $2, $3, $4, $4, $5
			) RETURNING id;`,
			model.EVENT_USER_MENTIONED, id, builder.submitterID,
			builder.ticket.PKID, commentID)
		if err := row.Scan(&eventID); err != nil {
			panic(err)
		}
		_, err := builder.tx.ExecContext(builder.ctx, `
			INSERT INTO event_notification (created, event_id, user_id)
			SELECT
				NOW() at time zone 'utc',
				$1, part.user_id
			FROM event_participant ev
			JOIN participant part ON part.id = ev.participant_id
			WHERE part.user_id IS NOT NULL AND ev.event_type = $2;
		`, eventID, model.EVENT_USER_MENTIONED)
		if err != nil {
			panic(err)
		}
	}

	for _, target := range builder.mentions.Tickets {
		_, err := builder.tx.ExecContext(builder.ctx, `
			WITH target AS (
				SELECT tk.id
				FROM ticket tk
				JOIN tracker tr ON tk.tracker_id = tr.id
				JOIN "user" u ON u.id = tr.owner_id
				WHERE u.username = $1 AND tr.name = $2 AND tk.scoped_id = $3
			)
			INSERT INTO event (
				created, event_type, by_participant_id, ticket_id,
				from_ticket_id, comment_id
			) VALUES (
				NOW() at time zone 'utc',
				$4, $5, (SELECT id FROM target), $6, $7
			)`,
			target.OwnerName, target.TrackerName, target.ID,
			model.EVENT_TICKET_MENTIONED, builder.submitterID,
			builder.ticket.PKID, commentID)
		if err != nil {
			panic(err)
		}
	}
	return builder
}

// Sets the given message ID to be added to In-Reply-To/References headers
func (builder *EventBuilder) InReplyTo(messageId *string) *EventBuilder {
	builder.inReplyTo = messageId
	return builder
}

func sendEmail(ctx context.Context, address, message string) error {
	return client.Do(ctx, "", "meta.sr.ht", client.GraphQLQuery{
		Query: `
		mutation sendEmail($address: String!, $message: String!) {
			sendEmail(address: $address, message: $message)
		}`,
		Variables: map[string]any{
			"address": address,
			"message": message,
		},
	}, struct{}{})
}

func (builder *EventBuilder) SendEmails(subject string,
	template *template.Template, context any) {
	var (
		submitterType          string
		submitterName          string
		submitterEmail         string
		notifySelf, copiedSelf bool
	)

	row := builder.tx.QueryRowContext(builder.ctx, `
		SELECT
			part.participant_type,
			CASE part.participant_type
			WHEN 'user' THEN "user".username
			WHEN 'email' THEN part.email_name
			ELSE '' END,
			CASE part.participant_type
			WHEN 'user' THEN "user".email
			WHEN 'email' THEN part.email
			ELSE '' END,
			CASE part.participant_type
			WHEN 'user' THEN "user".notify_self
			ELSE false END
		FROM participant part
		LEFT JOIN "user" ON "user".id = part.user_id
		WHERE part.id = $1
	`, builder.submitterID)
	if err := row.Scan(&submitterType, &submitterName, &submitterEmail, &notifySelf); err != nil {
		panic(err)
	}

	var body strings.Builder
	if err := template.Execute(&body, context); err != nil {
		panic(err)
	}

	conf := config.ForContext(builder.ctx)
	var notifyFrom string
	if addr, ok := conf.Get("todo.sr.ht", "notify-from"); ok {
		notifyFrom = addr
	} else if addr, ok := conf.Get("mail", "smtp-from"); ok {
		notifyFrom = addr
	} else {
		panic("Invalid mail configuration: missing todo.sr.ht::notify-from and fallback mail::smtp-from")
	}

	smtpUser, ok := conf.Get("mail", "smtp-user")
	if !ok {
		panic("Invalid mail configuration: missing mail::smtp-user")
	}

	postingDomain, ok := conf.Get("todo.sr.ht::mail", "posting-domain")
	if !ok {
		panic("Invalid mail configuration: missing todo.sr.ht::mail::posting-domain")
	}

	from := mail.Address{
		Name:    "~" + submitterName,
		Address: notifyFrom,
	}
	sender := mail.Address{
		Address: smtpUser + "@" + postingDomain,
	}

	ticketRef := builder.ticket.EmailRef(postingDomain)
	ticketAddress := mail.Address{
		Name:    builder.ticket.Ref(),
		Address: ticketRef,
	}

	// Generate the email, minus recipient
	var header mail.Header
	var message strings.Builder
	// TODO: List-Unsubscribe header
	header.SetAddressList("From", []*mail.Address{&from})
	header.SetAddressList("Reply-To", []*mail.Address{&ticketAddress})
	header.SetAddressList("Sender", []*mail.Address{&sender})
	if builder.eventType == model.EVENT_CREATED {
		if builder.inReplyTo != nil {
			header.SetMsgIDList("In-Reply-To", []string{*builder.inReplyTo})
		}
		header.SetMessageID(ticketRef)
	} else {
		refs := []string{ticketRef}
		if builder.inReplyTo != nil {
			refs = append(refs, *builder.inReplyTo)
		}
		header.SetMsgIDList("In-Reply-To", []string{ticketRef})
		header.SetMsgIDList("References", refs)
	}
	header.SetSubject(subject)
	msgBodyWriter, err := mail.CreateSingleInlineWriter(&message, header)
	if err != nil {
		panic(err)
	}
	_, err = io.WriteString(msgBodyWriter, body.String())
	if err != nil {
		panic(err)
	}
	msgBodyWriter.Close()

	// XXX: It may be possible to implement this more efficiently by skipping
	// the joins and pre-stashing the email details when inserting
	// event_participants.
	rows, err := builder.tx.QueryContext(builder.ctx, `
		SELECT
			part.participant_type,
			CASE part.participant_type
			WHEN 'user' THEN "user".username
			WHEN 'email' THEN COALESCE(part.email_name, '')
			ELSE '' END,
			CASE part.participant_type
			WHEN 'user' THEN "user".email
			WHEN 'email' THEN part.email
			ELSE '' END
		DISTINCT
		FROM event_participant evpart
		JOIN participant part ON evpart.participant_id = part.id
		LEFT JOIN "user" ON "user".id = part.user_id;
	`)
	if err != nil {
		panic(err)
	}

	set := make(map[string]any)
	for rows.Next() {
		var participantTypeString, name, address string
		if err := rows.Scan(&participantTypeString, &name, &address); err != nil {
			panic(err)
		}
		participantType := model.ParticipantTypeFromString(participantTypeString)
		if participantType == model.ParticipantTypeExternal {
			continue
		}
		if address == submitterEmail {
			if notifySelf {
				copiedSelf = true
			} else {
				continue
			}
		}
		if _, ok := set[address]; ok {
			continue
		}
		set[address] = nil

		to := mail.Address{
			Name:    name,
			Address: address,
		}
		if err := sendEmail(builder.ctx, to.String(), message.String()); err != nil {
			panic(err)
		}
	}
	if notifySelf && !copiedSelf {
		participantType := model.ParticipantTypeFromString(submitterType)
		if participantType != model.ParticipantTypeExternal {
			to := mail.Address{
				Name:    submitterName,
				Address: submitterEmail,
			}
			err := sendEmail(builder.ctx, to.String(), message.String())
			if err != nil {
				panic(err)
			}
		}
	}
}
