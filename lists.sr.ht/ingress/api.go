// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"bytes"
	"context"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/client"
	"git.sr.ht/~sircmpwn/core-go/config"
	"github.com/99designs/gqlgen/graphql"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

const (
	LISTS_SERVICE = "lists.sr.ht"
)

func GqlQueryUser(
	username string, query string, variables map[string]any, result any,
) error {
	ctx := config.Context(context.TODO(), SrhtConfig, LISTS_SERVICE)
	return client.Do(
		ctx, username, LISTS_SERVICE,
		client.GraphQLQuery{Query: query, Variables: variables},
		result,
	)
}

func parseListAddr(addr string) (owner, name string, cmd Command, err error) {
	cmd = CMD_POST

	// Note: we assume postfix took care of the domain
	listName, _, _ := strings.Cut(addr, "@")
	if i := strings.LastIndex(listName, "+"); i > 0 {
		cmd = Command(listName[i+1:])
		listName = listName[:i]
	}
	if redirect, ok := Config.Redirects[listName]; ok {
		listName = redirect
	}

	// split the list name into owner / listname
	if strings.HasPrefix(listName, "~") {
		var found bool
		owner, name, found = strings.Cut(listName, "/")
		if !found {
			err = &UnknownListError{addr}
			return
		}
		owner = strings.TrimPrefix(owner, "~")
	} else {
		// some mail providers do not allow "~" and "/" in addresses
		tokens := strings.Split(listName, ".")
		if len(tokens) < 3 || tokens[0] != "u" {
			err = &UnknownListError{addr}
			return
		}
		owner = tokens[1]
		name = strings.Join(tokens[2:], ".")
	}

	return
}

func LookupEmailDetails(msg *message.Entity, listAddr string) (*Sender, *MailingList, error) {
	fromAddr := msg.Header.Get("From")
	inReplyTo := msg.Header.Get("In-Reply-To")

	from, err := mail.ParseAddress(fromAddr)
	if err != nil {
		return nil, nil, err
	}
	owner, list, cmd, err := parseListAddr(listAddr)
	if err != nil {
		return nil, nil, err
	}

	var result struct {
		User *struct {
			List *struct {
				ID         int      `json:"id"`
				PermitMime []string `json:"permitMime"`
				RejectMime []string `json:"rejectMime"`
				ACL        Access   `json:"userACL"`
				ParentMsg  *struct {
					ID int `json:"id"`
				} `json:"message"`
			} `json:"list"`
		} `json:"user"`
	}

	err = GqlQueryUser(
		owner,
		`query($owner: String!, $list: String!, $from: String!, $msg: String!) {
			user(username: $owner) {
				list(name: $list) {
					id
					permitMime
					rejectMime
					userACL(email: $from) {
						browse
						reply
						post
						moderate
					}
					message(messageID: $msg) {
						id
					}
				}
			}
		}`,
		map[string]any{
			"owner": owner,
			"list":  list,
			"from":  from.Address,
			"msg":   inReplyTo,
		},
		&result,
	)
	if err != nil {
		return nil, nil, err
	}

	if result.User == nil || result.User.List == nil {
		return nil, nil, &UnknownListError{listAddr}
	}

	sender := &Sender{
		Name:  from.Name,
		Email: from.Address,
		ACL:   result.User.List.ACL,
	}

	mailingList := &MailingList{
		Owner:           owner,
		Name:            list,
		Command:         cmd,
		ID:              result.User.List.ID,
		PermitMimetypes: result.User.List.PermitMime,
		RejectMimetypes: result.User.List.RejectMime,
		IsReply:         result.User.List.ParentMsg != nil,
	}

	switch cmd {
	case CMD_SUBSCRIBE, CMD_UNSUBSCRIBE, CMD_CONFIRM_SUB, CMD_CONFIRM_UNSUB, CMD_POST:
		return sender, mailingList, nil
	default:
		return nil, nil, &UnknownCommandError{mailingList}
	}
}

func RequestSubscription(sender *Sender, list *MailingList) (string, error) {
	var result struct {
		Token string `json:"requestSubscription"`
	}
	err := GqlQueryUser(
		list.Owner,
		`mutation($list: Int!, $email: String!) {
			requestSubscription(listID: $list, email: $email)
		}`,
		map[string]any{"list": list.ID, "email": sender.Email},
		&result,
	)
	if err != nil {
		return "", err
	}
	return result.Token, nil
}

func ConfirmSubscription(sender *Sender, list *MailingList, token string) error {
	return GqlQueryUser(
		list.Owner,
		`mutation($token: ConfirmationToken!, $email: String!) {
			confirmSubscription(token: $token, email: $email) {
				id
			}
		}`,
		map[string]any{"token": token, "email": sender.Email},
		nil,
	)
}

func RequestUnsubscription(sender *Sender, list *MailingList) (string, error) {
	var result struct {
		Token string `json:"requestUnsubscription"`
	}
	err := GqlQueryUser(
		list.Owner,
		`mutation($list: Int!, $email: String!) {
			requestUnsubscription(listID: $list, email: $email)
		}`,
		map[string]any{"list": list.ID, "email": sender.Email},
		&result,
	)
	if err != nil {
		return "", err
	}
	return result.Token, nil
}

func ConfirmUnsubscription(sender *Sender, list *MailingList, token string) error {
	return GqlQueryUser(
		list.Owner,
		`mutation($token: ConfirmationToken!, $email: String!) {
			confirmUnsubscription(token: $token, email: $email) {
				id
			}
		}`,
		map[string]any{"token": token, "email": sender.Email},
		nil,
	)
}

func ArchiveMessage(data []byte, list *MailingList) error {
	ctx := config.Context(context.TODO(), SrhtConfig, LISTS_SERVICE)
	return client.Do(
		ctx, list.Owner, LISTS_SERVICE,
		client.GraphQLQuery{
			Query: `mutation($list: Int!, $msg: Upload!) {
				archiveMessage(listID: $list, message: $msg)
			}`,
			Variables: map[string]any{"list": list.ID},
			Uploads: map[string]graphql.Upload{
				"msg": {
					Filename:    "archive",
					File:        bytes.NewReader(data),
					ContentType: "message/rfc822",
				},
			},
		},
		nil,
	)
}

func LookupSubscribers(list *MailingList) ([]string, error) {
	var result struct {
		User struct {
			List struct {
				Subscriptions []struct {
					Subscriber struct {
						Email string `json:"email"`
					} `json:"subscriber"`
				} `json:"subscriptions"`
			} `json:"list"`
		} `json:"user"`
	}
	err := GqlQueryUser(
		list.Owner,
		`query($owner: String!, $list: String!) {
			user(username: $owner) {
				list(name: $list) {
					subscriptions {
						subscriber {
							... on Mailbox {
								email: address
							}
							... on User {
								email
							}
						}
					}
				}
			}
		}`,
		map[string]any{"owner": list.Owner, "list": list.Name},
		&result,
	)
	if err != nil {
		return nil, err
	}
	emails := make([]string, 0, len(result.User.List.Subscriptions))
	for _, s := range result.User.List.Subscriptions {
		emails = append(emails, s.Subscriber.Email)
	}
	return emails, nil
}

func CopySelf(address string) bool {
	var result struct {
		CopySelf bool `json:"copySelf"`
	}
	err := GqlQueryUser(
		"",
		`query($email: String!) {
			copySelf(email: $email)
		}`,
		map[string]any{"email": address},
		&result,
	)
	if err != nil {
		return false
	}
	return result.CopySelf
}
