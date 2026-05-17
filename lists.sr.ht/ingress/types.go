// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"fmt"
)

type Access struct {
	Browse   bool `json:"browse"`
	Reply    bool `json:"reply"`
	Post     bool `json:"post"`
	Moderate bool `json:"moderate"`
}

type Command string

const (
	CMD_SUBSCRIBE     Command = "subscribe"
	CMD_UNSUBSCRIBE   Command = "unsubscribe"
	CMD_CONFIRM_SUB   Command = "confirm-subscribe"
	CMD_CONFIRM_UNSUB Command = "confirm-unsubscribe"
	CMD_POST          Command = "post"
)

type MailingList struct {
	Owner           string
	Name            string
	ID              int
	PermitMimetypes []string
	RejectMimetypes []string
	Command         Command
	IsReply         bool
}

func (d *MailingList) FullName() string {
	return fmt.Sprintf("~%s/%s", d.Owner, d.Name)
}

func (d *MailingList) Address() string {
	return fmt.Sprintf("~%s/%s@%s", d.Owner, d.Name, Config.Domain)
}

func (d *MailingList) PlusAddress(cmd Command) string {
	return fmt.Sprintf("~%s/%s+%s@%s", d.Owner, d.Name, cmd, Config.Domain)
}

type Sender struct {
	Name  string
	Email string
	ACL   Access
}

type Patchset struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

type Email struct {
	ID        int       `json:"id"`
	MessageId string    `json:"-"`
	Patchset  *Patchset `json:"patchset"`
}
