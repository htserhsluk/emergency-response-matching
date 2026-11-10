package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/matching"
	"os"
)

func main() {
	inspect := flag.Bool("inspect-one", false, "inspect and requeue one delivery; does not allocate or acknowledge completion")
	flag.Parse()
	if !*inspect {
		fmt.Fprintln(os.Stderr, "Use --inspect-one; assignment worker is a subsequent milestone")
		os.Exit(2)
	}
	secret, address, url := os.Getenv("ERM_INTERNAL_SECRET"), os.Getenv("ERM_AMQP_URL"), os.Getenv("ERM_COORDINATOR_URL")
	if len(secret) < 32 || address == "" || url == "" {
		fmt.Fprintln(os.Stderr, "Broker/coordinator configuration required")
		os.Exit(2)
	}
	result, err := matching.InspectOne(context.Background(), address, "matching.work", httpapp.NewClient(url, secret))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
