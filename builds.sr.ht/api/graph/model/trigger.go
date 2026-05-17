package model

import (
	"encoding/json"

	"git.sr.ht/~sircmpwn/core-go/database"
)

const (
	TRIGGER_EMAIL   = "email"
	TRIGGER_WEBHOOK = "webhook"
)

type Trigger interface {
	IsTrigger()
}

type RawTrigger struct {
	ID          int
	Details     string
	Condition   string
	TriggerType string

	alias  string
	fields *database.ModelFields
}

func (t *RawTrigger) As(alias string) *RawTrigger {
	t.alias = alias
	return t
}

func (t *RawTrigger) Alias() string {
	return t.alias
}

func (t *RawTrigger) Table() string {
	return `"trigger"`
}

type EmailTrigger struct {
	ID        int              `json:"id"`
	To        string           `json:"to"`
	Cc        string           `json:"cc"`
	InReplyTo string           `json:"in_reply_to"`
	Condition TriggerCondition `json:"condition"`
}

func (EmailTrigger) IsTrigger() {}

type WebhookTrigger struct {
	ID        int              `json:"id"`
	URL       string           `json:"url"`
	Condition TriggerCondition `json:"condition"`
}

func (WebhookTrigger) IsTrigger() {}

func (t *RawTrigger) ToTrigger() Trigger {
	var cond TriggerCondition
	switch t.Condition {
	case "always":
		cond = TriggerConditionAlways
	case "failure":
		cond = TriggerConditionFailure
	case "success":
		cond = TriggerConditionSuccess
	default:
		panic("Database invariant broken: unknown trigger condition")
	}
	switch t.TriggerType {
	case TRIGGER_EMAIL:
		p := &EmailTrigger{
			ID:        t.ID,
			Condition: cond,
		}
		json.Unmarshal([]byte(t.Details), p)
		return p
	case TRIGGER_WEBHOOK:
		p := &WebhookTrigger{
			ID:        t.ID,
			Condition: cond,
		}
		json.Unmarshal([]byte(t.Details), p)
		return p
	default:
		panic("Database invariant broken: unknown trigger type")
	}
}

func (t *RawTrigger) Fields() *database.ModelFields {
	if t.fields != nil {
		return t.fields
	}
	t.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "condition", GQL: "condition", Ptr: &t.Condition},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &t.ID},
			{SQL: "details", GQL: "", Ptr: &t.Details},
			{SQL: "trigger_type", GQL: "", Ptr: &t.TriggerType},
		},
	}
	return t.fields
}
