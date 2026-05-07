package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

var (
	slackClient *slack.Client
	config      *Config
)

func initSlack() {
	slackClient = slack.New(config.SlackBotToken)
}

// HTTP handler: /slack/events
func handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Demo: bỏ verify signing secret; production nên verify theo docs Slack Events API.
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
		var r *slackevents.ChallengeResponse
		if err := json.Unmarshal(body, &r); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(r.Challenge))
		return

	case slackevents.CallbackEvent:
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			// Bỏ qua message do bot gửi để tránh loop
			if ev.BotID != "" {
				w.WriteHeader(http.StatusOK)
				return
			}
			go handleIncomingSlackMessage(ev)
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
	Metadata        map[string]interface{} `json:"metadata"`
}

func handleIncomingSlackMessage(ev *slackevents.MessageEvent) {
	metadata := map[string]interface{}{
		"source":           "slack",
		"slack_channel":    ev.Channel,
		"slack_thread_ts":  threadTS(ev),
		"slack_user_id":    ev.User,
		"slack_message_ts": ev.TimeStamp,
	}

	payload := CreateIssueRequest{
		Title:           "Slack message from " + ev.Channel,
		Issue:           ev.Text,
		AssigneeAgentID: config.IntakeAgentID,
		Metadata:        metadata,
	}

	b, _ := json.Marshal(payload)
	url := config.PaperclipBaseURL + "/api/issues" // cần chỉnh lại cho đúng API Paperclip của em
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		log.Println("create issue req error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.PaperclipAPIKey)

	resp, err := http.DefaultClient.Do(req)
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
