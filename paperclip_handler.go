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
	log.Printf("paperclip /webhook received: method=%s remote=%s ua=%q", r.Method, r.RemoteAddr, r.Header.Get("User-Agent"))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println("paperclip webhook read body error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("paperclip webhook body read: %d bytes", len(body))

	if !verifyPaperclipSignature(r.Header.Get("X-Paperclip-Signature"), body, config.PaperclipWebhookSecret) {
		log.Println("paperclip webhook signature verify failed")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	log.Println("paperclip webhook signature verified ok")

	var payload PaperclipWebhookPayload
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		log.Println("decode paperclip webhook error:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("paperclip webhook decoded: issue_id=%s decision=%s has_slack_reply=%t", payload.IssueID, payload.Output.Decision, payload.Output.SlackReply != nil)

	if payload.Output.SlackReply == nil || payload.Output.Decision != decisionReplyToSlack {
		log.Printf("paperclip webhook ignored: decision=%s slack_reply=%t (expected decision=%s)", payload.Output.Decision, payload.Output.SlackReply != nil, decisionReplyToSlack)
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
		log.Printf("paperclip webhook claimed issue_id=%s", payload.IssueID)
	} else {
		log.Println("paperclip webhook has empty issue_id; dedupe skipped")
	}

	// Pin reply destination to what the bridge originally recorded; only fall back to the
	// agent-supplied slack_reply fields if metadata is absent.
	ch := stringFromMeta(payload.Metadata, "slack_channel")
	chFromMeta := ch != ""
	if ch == "" {
		ch = payload.Output.SlackReply.Channel
	}
	ts := stringFromMeta(payload.Metadata, "slack_thread_ts")
	tsFromMeta := ts != ""
	if ts == "" {
		ts = payload.Output.SlackReply.ThreadTS
	}
	text := payload.Output.SlackReply.Text
	log.Printf("paperclip webhook reply target: channel=%s (from_meta=%t) thread=%s (from_meta=%t) text_len=%d", ch, chFromMeta, ts, tsFromMeta, len(text))

	if ch == "" || ts == "" {
		log.Println("paperclip webhook missing channel/thread_ts; skipping post")
		releaseIssue(payload.IssueID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("posting slack reply: channel=%s thread=%s", ch, ts)
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

	log.Println("slack reply sent to", ch, "thread", ts, "issue", payload.IssueID)
	w.WriteHeader(http.StatusOK)
}

func releaseIssue(id string) {
	if id == "" {
		return
	}
	processedIssuesMu.Lock()
	delete(processedIssues, id)
	processedIssuesMu.Unlock()
	log.Printf("paperclip webhook released issue_id=%s for retry", id)
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
