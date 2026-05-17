// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-smtp"
)

func StartSMTPServer(ctx context.Context) (net.Listener, *smtp.Server, error) {
	s := smtp.NewServer(&Backend{ctx})
	s.Addr = Config.Sock
	s.Domain = Config.Domain
	s.WriteTimeout = 10 * time.Second
	s.ReadTimeout = 10 * time.Second
	s.EnableSMTPUTF8 = true
	s.LMTP = Config.Protocol == "lmtp" || strings.Contains(Config.Sock, "/")
	s.ErrorLog = log.New(os.Stdout, "smtp/server: ", LogFlags)

	network := "tcp"
	if s.LMTP {
		network = "unix"
	}
	addr := s.Addr
	if !s.LMTP && addr == "" {
		addr = ":smtp"
	}
	l, err := net.Listen(network, addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen: %w", err)
	}
	if network == "unix" {
		path := l.Addr().String()
		if err := os.Chmod(path, 0o775); err != nil {
			return nil, nil, fmt.Errorf("chmod: %w", err)
		}
		group, err := user.LookupGroup(Config.SockGroup)
		if err != nil {
			return nil, nil, fmt.Errorf("user.LookupGroup: %w", err)
		}
		gid, err := strconv.ParseUint(group.Gid, 10, 16)
		if err != nil {
			return nil, nil, fmt.Errorf("strconv.ParseUint: %w", err)
		}
		if err := os.Chown(path, os.Getuid(), int(gid)); err != nil {
			return nil, nil, fmt.Errorf("chown: %w", err)
		}
	}

	return l, s, nil
}

var contentTypeParams = map[string]string{
	"charset": "utf-8",
	"format":  "flowed",
}

func ReplyHeaders(from, subject string, msg *message.Entity) mail.Header {
	header := new(mail.Header)
	header.Set("To", msg.Header.Get("From"))
	header.Set("From", from)
	header.Set("In-Reply-To", msg.Header.Get("Message-ID"))
	header.Set("References", msg.Header.Get("Message-ID"))
	header.SetSubject(subject)
	header.GenerateMessageIDWithHostname(Config.Domain)
	header.SetDate(time.Now())
	header.Set("Auto-Submitted", "auto-replied")
	header.SetContentType("text/plain", contentTypeParams)
	return *header
}

func SubjectFallback(msg *message.Entity, fallback string) string {
	subject, _ := msg.Header.Text("Subject")
	if subject == "" {
		subject = fallback
	}
	return subject
}
