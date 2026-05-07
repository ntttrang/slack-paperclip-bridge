package main

import (
	"log"
	"os"
)

type Config struct {
	SlackBotToken      string
	SlackSigningSecret string
	PaperclipBaseURL   string
	PaperclipAPIKey    string
	IntakeAgentID      string
	ListenAddr         string
}

func LoadConfig() *Config {
	cfg := &Config{
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
		PaperclipBaseURL:   os.Getenv("PAPERCLIP_BASE_URL"),
		PaperclipAPIKey:    os.Getenv("PAPERCLIP_API_KEY"),
		IntakeAgentID:      os.Getenv("INTAKE_AGENT_ID"),
		ListenAddr:         ":8080",
	}

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	if cfg.SlackBotToken == "" || cfg.PaperclipBaseURL == "" || cfg.PaperclipAPIKey == "" || cfg.IntakeAgentID == "" {
		log.Println("WARNING: some required env vars are empty. Check SLACK_BOT_TOKEN, PAPERCLIP_BASE_URL, PAPERCLIP_API_KEY, INTAKE_AGENT_ID")
	}
	return cfg
}
