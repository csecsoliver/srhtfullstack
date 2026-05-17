// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/email"
)

const LogFlags = log.Ldate | log.Ltime | log.Lshortfile

func main() {
	log.Default().SetOutput(os.Stdout)
	log.Default().SetPrefix("ingress: ")
	log.Default().SetFlags(LogFlags)

	if err := LoadConfig(); err != nil {
		log.Fatalf("config: %s", err)
	}

	// Subscribe to signals for graceful shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	ctx := config.Context(context.Background(), SrhtConfig, "ingress")

	egress := email.NewQueue(SrhtConfig)

	smtpSock, smtpSrv, err := StartSMTPServer(email.Context(ctx, egress))
	if err != nil {
		log.Fatalf("smtp: %s", err)
	}
	httpSock, httpSrv, err := StartPrometheusExporter()
	if err != nil {
		log.Fatalf("http: %s", err)
	}

	numWorkers := config.GetInt(SrhtConfig, "lists.sr.ht::worker", "queue-workers", 1)
	for range numWorkers {
		// Use a dedicated context per egress worker to hold persistent
		// SMTP client connections.
		go egress.Run(email.Context(ctx, egress))
	}

	go func() {
		log.Printf("Listening for incoming emails on %s://%s",
			Config.Protocol, Config.Sock)
		err = smtpSrv.Serve(smtpSock)
		// dummy signal here in case Serve() failed prematurely
		sig <- syscall.SIGCHLD
	}()
	go func() {
		log.Printf("Exposing metrics over http://[::]%s/metrics",
			Config.MetricsSock)
		if err := httpSrv.Serve(httpSock); err != nil {
			log.Printf("http.Serve: %s", err)
		}
	}()

	// Graceful shutdown.
	log.Printf("Received signal %v. Shutting down...", <-sig)
	// Wait until the SMTP/LMTP server has closed all connections.
	if e := smtpSrv.Shutdown(ctx); e != nil {
		log.Printf("smtp.Shutdown: %s", e)
	}
	if e := httpSrv.Shutdown(ctx); e != nil {
		log.Printf("http.Shutdown: %s", e)
	}

	// Wait until all egress workers have stopped.
	egress.Shutdown()

	if err != nil {
		log.Fatalf("smtp: %s", err)
	}
}
