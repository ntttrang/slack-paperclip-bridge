package main

import (
	"log"
	"os"
)

type Config struct {
	SlackBotToken          string
	SlackSigningSecret     string
	PaperclipBaseURL       string
	PaperclipAPIKey        string
	PaperclipWebhookSecret string
	IntakeAgentID          string
	ListenAddr             string
}

func LoadConfig() *Config {
	log.Println("loading config from environment")
	cfg := &Config{
		SlackBotToken:          os.Getenv("SLACK_BOT_TOKEN"),
		SlackSigningSecret:     os.Getenv("SLACK_SIGNING_SECRET"),
		PaperclipBaseURL:       os.Getenv("PAPERCLIP_BASE_URL"),
		PaperclipAPIKey:        os.Getenv("PAPERCLIP_API_KEY"),
		PaperclipWebhookSecret: os.Getenv("PAPERCLIP_WEBHOOK_SECRET"),
		IntakeAgentID:          os.Getenv("INTAKE_AGENT_ID"),
		ListenAddr:             ":8080",
	}

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	required := map[string]string{
		"SLACK_BOT_TOKEN":          cfg.SlackBotToken,
		"SLACK_SIGNING_SECRET":     cfg.SlackSigningSecret,
		"PAPERCLIP_BASE_URL":       cfg.PaperclipBaseURL,
		"PAPERCLIP_API_KEY":        cfg.PaperclipAPIKey,
		"PAPERCLIP_WEBHOOK_SECRET": cfg.PaperclipWebhookSecret,
		"INTAKE_AGENT_ID":          cfg.IntakeAgentID,
	}
	var missing []string
	for k, v := range required {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		log.Fatalf("missing required env vars: %v", missing)
	}
	log.Printf("config loaded: listen_addr=%s paperclip_base_url=%s intake_agent_id=%s", cfg.ListenAddr, cfg.PaperclipBaseURL, cfg.IntakeAgentID)
	return cfg
}
