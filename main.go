package main

import (
	"log"
	"net/http"
)

func main() {
	config = LoadConfig()
	initSlack()

	mux := http.NewServeMux()
	mux.HandleFunc("/slack/events", handleSlackEvents)
	mux.HandleFunc("/paperclip/webhook", handlePaperclipWebhook)

	srv := &http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	log.Println("Slack-Paperclip bridge listening on", config.ListenAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
