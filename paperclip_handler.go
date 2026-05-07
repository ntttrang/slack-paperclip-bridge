package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/slack-go/slack"
)

const decisionReplyToSlack = "reply_to_slack"

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
	Metadata map[string]any `json:"metadata"`
}

var (
	processedIssues   = map[string]struct{}{}
	processedIssuesMu sync.Mutex
)

// HTTP handler: /paperclip/webhook
func handlePaperclipWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !verifyPaperclipSignature(r.Header.Get("X-Paperclip-Signature"), body, config.PaperclipWebhookSecret) {
		log.Println("paperclip webhook signature verify failed")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var payload PaperclipWebhookPayload
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		log.Println("decode paperclip webhook error:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if payload.Output.SlackReply == nil || payload.Output.Decision != decisionReplyToSlack {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Claim the issue_id so concurrent retries are deduped. Rolled back below if Slack post fails.
	if payload.IssueID != "" {
		processedIssuesMu.Lock()
		if _, seen := processedIssues[payload.IssueID]; seen {
			processedIssuesMu.Unlock()
			log.Println("duplicate webhook for issue", payload.IssueID, "- skipping")
			w.WriteHeader(http.StatusOK)
			return
		}
		processedIssues[payload.IssueID] = struct{}{}
		processedIssuesMu.Unlock()
	}

	// Pin reply destination to what the bridge originally recorded; only fall back to the
	// agent-supplied slack_reply fields if metadata is absent.
	ch := stringFromMeta(payload.Metadata, "slack_channel")
	if ch == "" {
		ch = payload.Output.SlackReply.Channel
	}
	ts := stringFromMeta(payload.Metadata, "slack_thread_ts")
	if ts == "" {
		ts = payload.Output.SlackReply.ThreadTS
	}
	text := payload.Output.SlackReply.Text

	if ch == "" || ts == "" {
		log.Println("paperclip webhook missing channel/thread_ts; skipping post")
		releaseIssue(payload.IssueID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, _, err = slackClient.PostMessage(
		ch,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(ts),
	)
	if err != nil {
		log.Println("slack PostMessage error:", err)
		releaseIssue(payload.IssueID)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	log.Println("slack reply sent to", ch, "thread", ts)
	w.WriteHeader(http.StatusOK)
}

func releaseIssue(id string) {
	if id == "" {
		return
	}
	processedIssuesMu.Lock()
	delete(processedIssues, id)
	processedIssuesMu.Unlock()
}

func stringFromMeta(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func verifyPaperclipSignature(header string, body []byte, secret string) bool {
	if secret == "" || header == "" {
		return false
	}
	provided := strings.TrimPrefix(header, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(provided), []byte(expected))
}
