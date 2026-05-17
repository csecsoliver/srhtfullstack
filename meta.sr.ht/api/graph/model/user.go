package model

import (
	"context"
	"fmt"
	"path"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/objects"
)

type User struct {
	Bio              *string   `json:"bio"`
	Created          time.Time `json:"created"`
	Email            string    `json:"email"`
	ID               int       `json:"id"`
	Location         *string   `json:"location"`
	SuspensionNotice *string   `json:"suspensionNotice"`
	URL              *string   `json:"url"`
	Updated          time.Time `json:"updated"`
	UserType         UserType  `json:"userType"`
	Username         string    `json:"username"`
	Pronouns         *string   `json:"pronouns"`

	PGPKeyID   *int
	AvatarPath *string

	InternalPaymentStatus  PaymentStatus
	InternalPaymentDue     *time.Time
	DefaultPaymentMethodID *int

	alias  string
	fields *database.ModelFields
}

func (User) IsEntity() {}

func (u *User) CanonicalName() string {
	return "~" + u.Username
}

func (u *User) Avatar(ctx context.Context) *string {
	if u.AvatarPath == nil {
		return nil
	}
	conf := config.ForContext(ctx)
	bucket, _ := conf.Get("meta.sr.ht", "avatar-bucket")
	prefix, _ := conf.Get("meta.sr.ht", "avatar-prefix")
	filepath := path.Join(bucket, prefix, *u.AvatarPath)
	path := fmt.Sprintf("%s/%s", objects.URL(conf), filepath)
	return &path
}

func (u *User) As(alias string) *User {
	u.alias = alias
	return u
}

func (u *User) Alias() string {
	return u.alias
}

func (u *User) Table() string {
	return `"user"`
}

func (u *User) Fields() *database.ModelFields {
	if u.fields != nil {
		return u.fields
	}
	u.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &u.ID},
			{SQL: "created", GQL: "created", Ptr: &u.Created},
			{SQL: "updated", GQL: "updated", Ptr: &u.Updated},
			{SQL: "username", GQL: "username", Ptr: &u.Username},
			{SQL: "email", GQL: "email", Ptr: &u.Email},
			{SQL: "url", GQL: "url", Ptr: &u.URL},
			{SQL: "location", GQL: "location", Ptr: &u.Location},
			{SQL: "bio", GQL: "bio", Ptr: &u.Bio},
			{SQL: "suspension_notice", GQL: "suspensionNotice", Ptr: &u.SuspensionNotice},
			{SQL: "payment_status", GQL: "paymentStatus", Ptr: &u.InternalPaymentStatus},
			{SQL: "payment_due", GQL: "paymentDue", Ptr: &u.InternalPaymentDue},
			{SQL: "pronouns", GQL: "pronouns", Ptr: &u.Pronouns},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &u.ID},
			{SQL: "username", GQL: "", Ptr: &u.Username},
			{SQL: "user_type", GQL: "", Ptr: &u.UserType},
			{SQL: "pgp_key_id", GQL: "", Ptr: &u.PGPKeyID},
			{SQL: "default_payment_method_id", GQL: "", Ptr: &u.DefaultPaymentMethodID},
			{SQL: "avatar", GQL: "", Ptr: &u.AvatarPath},
		},
	}
	return u.fields
}
