package trackers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/crypto"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/loaders"
)

type TrackerDump struct {
	ID          int       `json:"id"`
	Owner       User      `json:"owner"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Labels      []Label   `json:"labels,omitempty"`
	Tickets     []Ticket  `json:"tickets,omitempty"`
}

type Label struct {
	ID              int       `json:"id"`
	Created         time.Time `json:"created"`
	Name            string    `json:"name"`
	BackgroundColor string    `json:"background_color"`
	ForegroundColor string    `json:"foreground_color"`
}

type Ticket struct {
	ID         int         `json:"id"`
	Created    time.Time   `json:"created"`
	Updated    time.Time   `json:"updated"`
	Submitter  Participant `json:"submitter"`
	Ref        string      `json:"ref"`
	Subject    string      `json:"subject"`
	Body       string      `json:"body,omitempty"`
	Status     string      `json:"status"`
	Resolution string      `json:"resolution"`
	Labels     []string    `json:"labels,omitempty"`
	Assignees  []User      `json:"assignees,omitempty"`
	Upstream   string      `json:"upstream"`
	Signature  string      `json:"X-Payload-Signature,omitempty"`
	Nonce      string      `json:"X-Payload-Nonce,omitempty"`
	Events     []Event     `json:"events,omitempty"`
}

type TrackerRef struct {
	Owner   User      `json:"owner"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Name    string    `json:"name"`
}

type TicketRef struct {
	ID      int        `json:"id"`
	Ref     string     `json:"ref"`
	Tracker TrackerRef `json:"tracker"`
}

type Event struct {
	ID            int          `json:"id"`
	Created       time.Time    `json:"created"`
	EventType     []string     `json:"event_type"`
	OldStatus     string       `json:"old_status,omitempty"`
	OldResolution string       `json:"old_resolution,omitempty"`
	NewStatus     string       `json:"new_status,omitempty"`
	NewResolution string       `json:"new_resolution,omitempty"`
	Participant   *Participant `json:"participant,omitempty"`
	Comment       *Comment     `json:"comment,omitempty"`
	Label         *string      `json:"label,omitempty"`
	ByUser        *Participant `json:"by_user,omitempty"`
	FromTicket    *TicketRef   `json:"from_ticket,omitempty"`
	Upstream      string       `json:"upstream"`
	Signature     string       `json:"X-Payload-Signature,omitempty"`
	Nonce         string       `json:"X-Payload-Nonce,omitempty"`
}

type Participant struct {
	Type          string `json:"type"`
	UserID        int    `json:"user_id,omitempty"`
	CanonicalName string `json:"canonical_name,omitempty"`
	Name          string `json:"name,omitempty"`
	Address       string `json:"address,omitempty"`
	ExternalID    string `json:"external_id,omitempty"`
	ExternalURL   string `json:"external_url,omitempty"`
}

type User struct {
	ID            int    `json:"id"`
	CanonicalName string `json:"canonical_name"`
	Name          string `json:"name"`
}

type Comment struct {
	ID      int         `json:"id"`
	Created time.Time   `json:"created"`
	Author  Participant `json:"author"`
	Text    string      `json:"text"`
}

type TicketSignatureData struct {
	TrackerID   int    `json:"tracker_id"`
	TicketID    int    `json:"ticket_id"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	SubmitterID int    `json:"submitter_id"`
	Upstream    string `json:"upstream"`
}

type CommentSignatureData struct {
	TrackerID int    `json:"tracker_id"`
	TicketID  int    `json:"ticket_id"`
	Comment   string `json:"comment"`
	AuthorID  int    `json:"author_id"`
	Upstream  string `json:"upstream"`
}

func importParticipant(ctx context.Context, part Participant, upstream, ourUpstream string) (int, error) {
	switch part.Type {
	case "user":
		if upstream == ourUpstream {
			part, err := loaders.ForContext(ctx).ParticipantsByUsername.Load(part.Name)
			if err == nil {
				return part.ID, nil
			}
		}
		return importExternalParticipant(ctx, part.CanonicalName,
			upstream+"/"+part.CanonicalName)
	case "email":
		// TODO: check if the email is registered on this upstream?
		return importEmailParticipant(ctx, part.Address, part.Name)
	case "external":
		// TODO: check if the user is registered on this upstream?
		return importExternalParticipant(ctx, part.ExternalID, part.ExternalURL)
	default:
		return 0, fmt.Errorf("invalid participant type %q", part.Type)
	}
}

func importEmailParticipant(ctx context.Context, address, name string) (int, error) {
	var partID int
	err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO participant (
				created, participant_type, email, email_name
			) VALUES (
				NOW() at time zone 'utc',
				'email',
				$1, $2
			)
			ON CONFLICT ON CONSTRAINT participant_email_key
			DO UPDATE SET created = participant.created
			RETURNING id
		`, address, name)
		return row.Scan(&partID)
	})
	if err != nil {
		return 0, err
	}
	return partID, nil
}

func importExternalParticipant(ctx context.Context, id, url string) (int, error) {
	var partID int
	err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			INSERT INTO participant (
				created, participant_type, external_id, external_url
			) VALUES (
				NOW() at time zone 'utc',
				'external',
				$1, $2
			)
			ON CONFLICT ON CONSTRAINT participant_external_id_key
			DO UPDATE SET created = participant.created
			RETURNING id
		`, id, url)
		return row.Scan(&partID)
	})
	if err != nil {
		return 0, err
	}
	return partID, nil
}

func importTrackerDump(ctx context.Context, trackerID int, dump io.Reader, ourUpstream string) error {
	defer func() {
		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				UPDATE tracker
				SET import_in_progress = false
				WHERE id = $1
			`, trackerID)
			return err
		}); err != nil {
			panic(err)
		}
	}()

	var tracker TrackerDump
	if err := json.NewDecoder(dump).Decode(&tracker); err != nil {
		return err
	}

	if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
		// Create labels
		labelIDs := map[string]int{}
		for _, label := range tracker.Labels {
			row := tx.QueryRowContext(ctx, `
				INSERT INTO label (
					created, updated, tracker_id, name, color, text_color
				) VALUES (
					$1, $1, $2, $3, $4, $5
				) RETURNING id
			`, label.Created, trackerID, label.Name,
				label.BackgroundColor, label.ForegroundColor)
			var labelID int
			if err := row.Scan(&labelID); err != nil {
				return err
			}
			labelIDs[label.Name] = labelID
		}

		var nextTicketID int
		row := tx.QueryRowContext(ctx,
			`SELECT next_ticket_id FROM tracker WHERE id = $1`,
			trackerID)
		if err := row.Scan(&nextTicketID); err != nil {
			return err
		}
		// Make sure that the tracker does not have any existing tickets
		// to avoid conflicts.
		if nextTicketID != 1 {
			return errors.New("tracker must not have any existing tickets")
		}

		var maxTicketID int

		for _, ticket := range tracker.Tickets {
			submitterID, err := importParticipant(ctx, ticket.Submitter, ticket.Upstream, ourUpstream)
			if err != nil {
				return err
			}

			ticketAuthenticity := model.AuthenticityUnauthenticated
			if ticket.Upstream == ourUpstream && ticket.Submitter.Type == "user" {
				sigdata := TicketSignatureData{
					TrackerID:   tracker.ID,
					TicketID:    ticket.ID,
					Subject:     ticket.Subject,
					Body:        ticket.Body,
					SubmitterID: ticket.Submitter.UserID,
					Upstream:    ticket.Upstream,
				}
				payload, err := json.Marshal(sigdata)
				if err != nil {
					panic(err)
				}
				if crypto.VerifyWebhook(payload, ticket.Nonce, ticket.Signature) {
					ticketAuthenticity = model.AuthenticityAuthentic
				} else {
					ticketAuthenticity = model.AuthenticityTampered
				}
			}

			// Compute the max ticket ID. We can't use the number of tickets as
			// the next ticket ID because that won't include deleted tickets
			if ticket.ID > maxTicketID {
				maxTicketID = ticket.ID
			}
			// We don't need to check for existing tickets since we ensured that
			// the tracker has no tickets.
			row := tx.QueryRowContext(ctx, `
				INSERT INTO ticket (
					created, updated,
					tracker_id, scoped_id,
					submitter_id, title, description,
					status, resolution, authenticity
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
				)
				RETURNING id
			`, ticket.Created, ticket.Updated, trackerID, ticket.ID,
				submitterID, ticket.Subject, ticket.Body,
				model.TicketStatus(strings.ToUpper(ticket.Status)),
				model.TicketResolution(strings.ToUpper(ticket.Resolution)),
				ticketAuthenticity)
			var ticketPKID int
			if err := row.Scan(&ticketPKID); err != nil {
				return err
			}

			for _, label := range ticket.Labels {
				_, err := tx.ExecContext(ctx, `
					INSERT INTO ticket_label (
						created, ticket_id, label_id, user_id
					) VALUES (
						NOW() at time zone 'utc',
						$1, $2,
						(SELECT owner_id FROM tracker WHERE id = $3)
					)
				`, ticketPKID, labelIDs[label], trackerID)
				if err != nil {
					return err
				}
			}

			// TODO: assignees

			for _, event := range ticket.Events {
				var (
					commentID       *int
					labelID         *int
					partID          *int
					oldStatus       *model.TicketStatus
					newStatus       *model.TicketStatus
					oldResolution   *model.TicketResolution
					newResolution   *model.TicketResolution
					byParticipantID *int
				)

				var eventType int
				for _, etype := range event.EventType {
					etype := model.EventType(etype)
					if !etype.IsValid() {
						return fmt.Errorf("failed to import ticket #%d: invalid event type %s", ticket.ID, etype)
					}
					eventType |= eventTypeMap[etype]
				}
				if eventType == 0 {
					return fmt.Errorf("failed to import ticket #%d: invalid ticket event", ticket.ID)
				}

				if event.Participant != nil {
					eventPartID, err := importParticipant(ctx, *event.Participant, event.Upstream, ourUpstream)
					if err != nil {
						return err
					}
					partID = &eventPartID
				}

				if eventType&model.EVENT_COMMENT != 0 {
					authorID, err := importParticipant(ctx, event.Comment.Author, event.Upstream, ourUpstream)
					if err != nil {
						return err
					}

					commentAuthenticity := model.AuthenticityUnauthenticated
					if event.Upstream == ourUpstream && event.Comment.Author.Type == "user" {
						log.Println(event.Comment.Author.UserID)
						sigdata := CommentSignatureData{
							TrackerID: tracker.ID,
							TicketID:  ticket.ID,
							Comment:   event.Comment.Text,
							AuthorID:  event.Comment.Author.UserID,
							Upstream:  event.Upstream,
						}
						payload, err := json.Marshal(sigdata)
						if err != nil {
							panic(err)
						}
						if crypto.VerifyWebhook(payload, event.Nonce, event.Signature) {
							commentAuthenticity = model.AuthenticityAuthentic
						} else {
							commentAuthenticity = model.AuthenticityTampered
						}
					}

					row := tx.QueryRowContext(ctx, `
						INSERT INTO ticket_comment (
							created, updated, submitter_id, ticket_id, text,
							authenticity
						) VALUES (
							$1, $1, $2, $3, $4, $5
						) RETURNING id
					`, event.Comment.Created, authorID, ticketPKID, event.Comment.Text,
						commentAuthenticity)
					var _commentID int
					if err := row.Scan(&_commentID); err != nil {
						return err
					}
					commentID = &_commentID

					_, err = tx.ExecContext(ctx, `
						UPDATE ticket
						SET comment_count = comment_count + 1
						WHERE id = $1
					`, ticketPKID)
					if err != nil {
						return err
					}
				}
				if eventType&model.EVENT_STATUS_CHANGE != 0 {
					_oldStatus := model.TicketStatus(strings.ToUpper(event.OldStatus))
					_newStatus := model.TicketStatus(strings.ToUpper(event.NewStatus))
					_oldResolution := model.TicketResolution(strings.ToUpper(event.OldResolution))
					_newResolution := model.TicketResolution(strings.ToUpper(event.NewResolution))
					oldStatus = &_oldStatus
					newStatus = &_newStatus
					oldResolution = &_oldResolution
					newResolution = &_newResolution
				}
				if eventType&model.EVENT_LABEL_ADDED != 0 ||
					eventType&model.EVENT_LABEL_REMOVED != 0 {
					_labelID := labelIDs[*event.Label]
					labelID = &_labelID
				}
				if eventType&model.EVENT_ASSIGNED_USER != 0 ||
					eventType&model.EVENT_UNASSIGNED_USER != 0 {
					partID, err := importParticipant(ctx, *event.ByUser, event.Upstream, ourUpstream)
					if err != nil {
						return err
					}
					byParticipantID = &partID
				}
				if eventType&model.EVENT_USER_MENTIONED != 0 {
					// Magic event type, do not import
					continue
				}
				if eventType&model.EVENT_TICKET_MENTIONED != 0 {
					// TODO: Could reference tickets imported in later iterations
					continue
				}

				_, err := tx.ExecContext(ctx, `
					INSERT INTO event (
						created, event_type, participant_id, ticket_id,
						old_status, new_status, old_resolution, new_resolution,
						comment_id, label_id, by_participant_id
					) VALUES (
						$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
					)
				`, event.Created, eventType, partID, ticketPKID,
					oldStatus, newStatus, oldResolution, newResolution,
					commentID, labelID, byParticipantID)
				if err != nil {
					return err
				}
			}
		}

		// Update tracker.next_ticket_id
		if maxTicketID != 0 {
			if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `
					UPDATE tracker
					SET next_ticket_id = $2 + 1
					WHERE id = $1
				`, trackerID, maxTicketID)
				return err
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

var eventTypeMap = map[model.EventType]int{
	model.EventTypeCreated:         model.EVENT_CREATED,
	model.EventTypeComment:         model.EVENT_COMMENT,
	model.EventTypeStatusChange:    model.EVENT_STATUS_CHANGE,
	model.EventTypeLabelAdded:      model.EVENT_LABEL_ADDED,
	model.EventTypeLabelRemoved:    model.EVENT_LABEL_REMOVED,
	model.EventTypeAssignedUser:    model.EVENT_ASSIGNED_USER,
	model.EventTypeUnassignedUser:  model.EVENT_UNASSIGNED_USER,
	model.EventTypeUserMentioned:   model.EVENT_USER_MENTIONED,
	model.EventTypeTicketMentioned: model.EVENT_TICKET_MENTIONED,
}
