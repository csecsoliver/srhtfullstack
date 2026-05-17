// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2024 Robin Jarry

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// prometheus counters

var RejectedCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "conn_rejected",
	Help:      "Total number of rejected connections.",
})

var DroppedCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "emails_dropped",
	Help:      "Total number of silently dropped messages.",
})

var EmailsCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "emails_received",
	Help:      "Total number of emails received.",
})

var ErrorsCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "email_errors",
	Help:      "Total number of erroneous emails received.",
})

var BounceCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "email_bounced",
	Help:      "Total number of bounced emails.",
})

var ForwardsCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "forwards_processed",
	Help:      "Total number of emails forwarded to subscribers.",
})

var CommandsCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "srht",
	Subsystem: "lists",
	Name:      "commands_processed",
	Help:      "Total number of commands processed, e.g. +subscribe.",
})

func StartPrometheusExporter() (net.Listener, *http.Server, error) {
	listener, err := net.Listen("tcp", Config.MetricsSock)
	if err != nil {
		return nil, nil, fmt.Errorf("listen: %w", err)
	}

	prometheus.MustRegister(
		RejectedCounter,
		EmailsCounter,
		ErrorsCounter,
		BounceCounter,
		ForwardsCounter,
		CommandsCounter,
	)
	handler := promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	)
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)

	server := &http.Server{
		Handler:  mux,
		Addr:     Config.MetricsSock,
		ErrorLog: log.New(os.Stdout, "http/server: ", LogFlags),
	}

	return listener, server, nil
}
