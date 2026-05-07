package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var inflight sync.WaitGroup

func main() {
	config = LoadConfig()
	initSlack()

	mux := http.NewServeMux()
	mux.HandleFunc("/slack/events", handleSlackEvents)
	mux.HandleFunc("/paperclip/webhook", handlePaperclipWebhook)

	srv := &http.Server{
		Addr:              config.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Println("Slack-Paperclip bridge listening on", config.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown signal received; draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("server shutdown error:", err)
	}
	inflight.Wait()
	log.Println("drained; exiting")
}
