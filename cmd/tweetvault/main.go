// Command tweetvault runs the tweet vault's HTTP API.
package main

import (
	"flag"
	"log"
	"net/http"

	"tweetvault/internal/api"
	"tweetvault/internal/store"
)

func main() {
	dataFile := flag.String("data", "tweetvault.json", "path to the JSON data file")
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	s, err := store.Load(*dataFile)
	if err != nil {
		log.Fatalf("loading store from %s: %v", *dataFile, err)
	}

	server := api.New(s)
	log.Printf("tweetvault listening on %s (data: %s)", *addr, *dataFile)
	if err := http.ListenAndServe(*addr, server.Router()); err != nil {
		log.Fatal(err)
	}
}
