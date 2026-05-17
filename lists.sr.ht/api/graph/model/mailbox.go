package model

import "fmt"

type Mailbox struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (Mailbox) IsEntity() {}

func (mb Mailbox) CanonicalName() string {
	return fmt.Sprintf("%s <%s>", mb.Name, mb.Address)
}
