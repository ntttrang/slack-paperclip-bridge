package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/slack-go/slack"
)

// Struct dùng nhận webhook từ Paperclip
type SlackReply struct {
	Channel  string `json:"channel"`
	ThreadTS string `json:"thread_ts"`
	Text     string `json:"text"`
}

type PaperclipWebhookPayload struct {
	IssueID string `json:"issue_id"`
	Output  struct {
		Decision   string      `json:"decision"`
		SlackReply *SlackReply `json:"slack_reply"`
	} `json:"output"`
	Metadata map[string]interface{} `json:"metadata"`
}

// HTTP handler: /paperclip/webhook
func handlePaperclipWebhook(w http.ResponseWriter, r *http.Request) {
	var payload PaperclipWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Println("decode paperclip webhook error:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if payload.Output.SlackReply == nil || payload.Output.Decision != "reply_to_slack" {
		w.WriteHeader(http.StatusOK)
		return
	}

	reply := payload.Output.SlackReply
	ch := reply.Channel
	ts := reply.ThreadTS
	text := reply.Text

	_, _, err := slackClient.PostMessage(
		ch,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(ts),
	)
	if err != nil {
		log.Println("slack PostMessage error:", err)
	} else {
		log.Println("slack reply sent to", ch, "thread", ts)
	}

	w.WriteHeader(http.StatusOK)
}
