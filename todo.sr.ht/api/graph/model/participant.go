package model

import "fmt"

// Used internally
type Participant struct {
	ID int
	// Note: Right now we don't need any other fields
}

type ParticipantType string

const (
	ParticipantTypeUser     ParticipantType = "user"
	ParticipantTypeEmail    ParticipantType = "email"
	ParticipantTypeExternal ParticipantType = "external"
)

func ParticipantTypeFromString(participantType string) ParticipantType {
	switch participantType {
	case "user":
		return ParticipantTypeUser
	case "email":
		return ParticipantTypeEmail
	case "external":
		return ParticipantTypeExternal
	default:
		panic("database invariant broken")
	}
}

type EmailAddress struct {
	Mailbox string  `json:"mailbox"`
	Name    *string `json:"name"`
}

func (EmailAddress) IsEntity() {}

func (e EmailAddress) CanonicalName() string {
	if e.Name != nil {
		return fmt.Sprintf("%s <%s>", *e.Name, e.Mailbox)
	}
	return e.Mailbox
}

type ExternalUser struct {
	ExternalID  string  `json:"externalId"`
	ExternalURL *string `json:"externalUrl"`
}

func (ExternalUser) IsEntity() {}

func (e ExternalUser) CanonicalName() string {
	return e.ExternalID
}
