package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const sourceSlack = "slack"

var (
	slackClient     *slack.Client
	config          *Config
	paperclipClient = &http.Client{Timeout: 15 * time.Second}
)

func initSlack() {
	log.Println("initializing slack client")
	slackClient = slack.New(config.SlackBotToken)
	log.Println("slack client initialized")
}

// HTTP handler: /slack/events
func handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	log.Printf("slack /events received: method=%s remote=%s ua=%q", r.Method, r.RemoteAddr, r.Header.Get("User-Agent"))

	// Slack retries up to 3x if we don't ack within 3s. Ack retries without re-processing
	// to avoid duplicate Paperclip issues.
	if retry := r.Header.Get("X-Slack-Retry-Num"); retry != "" {
		log.Printf("slack retry detected (num=%s reason=%s); acking without reprocessing", retry, r.Header.Get("X-Slack-Retry-Reason"))
		w.WriteHeader(http.StatusOK)
		return
	}

	sv, err := slack.NewSecretsVerifier(r.Header, config.SlackSigningSecret)
	if err != nil {
		log.Println("slack secrets verifier init error:", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.TeeReader(r.Body, &sv))
	if err != nil {
		log.Println("slack read body error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("slack body read: %d bytes", len(body))

	if err := sv.Ensure(); err != nil {
		log.Println("slack signature verify failed:", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	log.Println("slack signature verified ok")

	eventsAPIEvent, err := slackevents.ParseEvent(
		json.RawMessage(body),
		slackevents.OptionNoVerifyToken(),
	)
	if err != nil {
		log.Println("slackevents.ParseEvent error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("slack event parsed: type=%s", eventsAPIEvent.Type)

	switch eventsAPIEvent.Type {
	case slackevents.URLVerification:
		log.Println("slack url_verification challenge received")
		var resp *slackevents.ChallengeResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			log.Println("slack url_verification unmarshal error:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(resp.Challenge))
		log.Println("slack url_verification challenge responded")
		return

	case slackevents.CallbackEvent:
		innerEvent := eventsAPIEvent.InnerEvent
		log.Printf("slack callback inner event type=%s", innerEvent.Type)
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			// Skip bot messages, edits/deletes/joins (SubType set), and events without a user.
			if ev.BotID != "" || ev.SubType != "" || ev.User == "" {
				log.Printf("slack message skipped: bot_id=%q subtype=%q user=%q channel=%s", ev.BotID, ev.SubType, ev.User, ev.Channel)
				w.WriteHeader(http.StatusOK)
				return
			}
			log.Printf("slack message accepted: channel=%s user=%s ts=%s thread_ts=%s text_len=%d", ev.Channel, ev.User, ev.TimeStamp, threadTS(ev), len(ev.Text))
			inflight.Go(func() {
				defer func() {
					if rec := recover(); rec != nil {
						log.Println("panic in handleIncomingSlackMessage:", rec)
					}
				}()
				handleIncomingSlackMessage(ev)
			})
			w.WriteHeader(http.StatusOK)
			return
		default:
			log.Printf("slack callback inner event ignored: type=%T", innerEvent.Data)
			w.WriteHeader(http.StatusOK)
			return
		}

	default:
		log.Printf("slack event type ignored: %s", eventsAPIEvent.Type)
		w.WriteHeader(http.StatusOK)
		return
	}
}

// Request gửi lên Paperclip
type CreateIssueRequest struct {
	Title           string                 `json:"title"`
	Issue           string                 `json:"issue"`
	AssigneeAgentID string                 `json:"assigneeAgentId"`
	Metadata        map[string]any `json:"metadata"`
}

func handleIncomingSlackMessage(ev *slackevents.MessageEvent) {
	log.Printf("handleIncomingSlackMessage start: channel=%s user=%s ts=%s", ev.Channel, ev.User, ev.TimeStamp)

	metadata := map[string]any{
		"source":           sourceSlack,
		"slack_channel":    ev.Channel,
		"slack_thread_ts":  threadTS(ev),
		"slack_user_id":    ev.User,
		"slack_message_ts": ev.TimeStamp,
	}

	title := fmt.Sprintf("Slack: %.80s", ev.Text)
	if title == "Slack: " {
		title = "Slack message from " + ev.Channel
		log.Printf("slack message has empty text; using fallback title")
	}

	payload := CreateIssueRequest{
		Title:           title,
		Issue:           ev.Text,
		AssigneeAgentID: config.IntakeAgentID,
		Metadata:        metadata,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		log.Println("marshal create issue payload error:", err)
		return
	}
	log.Printf("paperclip create issue payload prepared: title=%q issue_len=%d agent=%s", title, len(ev.Text), config.IntakeAgentID)

	// TODO: confirm the real Paperclip Intake API path & field names; this is a placeholder.
	url := config.PaperclipBaseURL + "/api/issues"
	log.Printf("paperclip create issue POST -> %s", url)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		log.Println("create issue req error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.PaperclipAPIKey)

	start := time.Now()
	resp, err := paperclipClient.Do(req)
	if err != nil {
		log.Printf("call paperclip error after %s: %v", time.Since(start), err)
		return
	}
	defer resp.Body.Close()
	log.Printf("paperclip create issue response: status=%d duration=%s", resp.StatusCode, time.Since(start))

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Println("paperclip create issue non-2xx:", resp.StatusCode, string(bodyBytes))
	} else {
		log.Println("paperclip issue created ok for channel", ev.Channel, "thread", threadTS(ev))
	}
}

func threadTS(ev *slackevents.MessageEvent) string {
	if ev.ThreadTimeStamp != "" {
		return ev.ThreadTimeStamp
	}
	return ev.TimeStamp
}
