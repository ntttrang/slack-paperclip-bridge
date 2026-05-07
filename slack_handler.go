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
	slackClient = slack.New(config.SlackBotToken)
}

// HTTP handler: /slack/events
func handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	// Slack retries up to 3x if we don't ack within 3s. Ack retries without re-processing
	// to avoid duplicate Paperclip issues.
	if r.Header.Get("X-Slack-Retry-Num") != "" {
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := sv.Ensure(); err != nil {
		log.Println("slack signature verify failed:", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	eventsAPIEvent, err := slackevents.ParseEvent(
		json.RawMessage(body),
		slackevents.OptionNoVerifyToken(),
	)
	if err != nil {
		log.Println("slackevents.ParseEvent error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch eventsAPIEvent.Type {
	case slackevents.URLVerification:
		var resp *slackevents.ChallengeResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(resp.Challenge))
		return

	case slackevents.CallbackEvent:
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			// Skip bot messages, edits/deletes/joins (SubType set), and events without a user.
			if ev.BotID != "" || ev.SubType != "" || ev.User == "" {
				w.WriteHeader(http.StatusOK)
				return
			}
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
			w.WriteHeader(http.StatusOK)
			return
		}

	default:
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

	// TODO: confirm the real Paperclip Intake API path & field names; this is a placeholder.
	url := config.PaperclipBaseURL + "/api/issues"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		log.Println("create issue req error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.PaperclipAPIKey)

	resp, err := paperclipClient.Do(req)
	if err != nil {
		log.Println("call paperclip error:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Println("paperclip create issue non-2xx:", resp.StatusCode, string(bodyBytes))
	} else {
		log.Println("paperclip issue created ok for channel", ev.Channel)
	}
}

func threadTS(ev *slackevents.MessageEvent) string {
	if ev.ThreadTimeStamp != "" {
		return ev.ThreadTimeStamp
	}
	return ev.TimeStamp
}
