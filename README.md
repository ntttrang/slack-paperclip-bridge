# slack-paperclip-bridge

A lightweight Go HTTP service that bridges Slack and Paperclip:

- Forwards Slack messages into Paperclip as new issues assigned to a configured intake agent.
- Receives webhooks from Paperclip and posts replies back into the originating Slack thread.

## Endpoints

- `POST /slack/events` — Slack Events API receiver (URL verification + `message` events).
- `POST /paperclip/webhook` — Paperclip webhook receiver. Posts back to Slack when `output.decision == "reply_to_slack"` and `output.slack_reply` is present.

## Configuration

The service reads configuration from environment variables (see `.env.example`):

| Variable | Required | Description |
| --- | --- | --- |
| `SLACK_BOT_TOKEN` | yes | Slack bot user OAuth token (`xoxb-...`). |
| `SLACK_SIGNING_SECRET` | yes | Slack signing secret. Used to verify `X-Slack-Signature` on every event. |
| `PAPERCLIP_BASE_URL` | yes | Base URL of the Paperclip API. |
| `PAPERCLIP_API_KEY` | yes | Bearer token for the Paperclip API. |
| `PAPERCLIP_WEBHOOK_SECRET` | yes | Shared secret used to verify `X-Paperclip-Signature` (`sha256=<hex hmac>` of the raw body). |
| `INTAKE_AGENT_ID` | yes | Paperclip agent ID that incoming Slack issues are assigned to. |
| `LISTEN_ADDR` | no | HTTP listen address. Defaults to `:8080`. |

The bridge fails fast at startup if any required variable is missing.

Copy `.env.example` to `.env` and fill in the values, then export them into your shell before running.

## Running

```sh
go run .
```

The server logs `Slack-Paperclip bridge listening on <addr>` on startup.

## Project layout

- `main.go` — wires up the HTTP server and routes.
- `config.go` — loads configuration from environment variables.
- `slack_handler.go` — handles incoming Slack events and creates Paperclip issues.
- `paperclip_handler.go` — handles Paperclip webhooks and posts replies to Slack.

## Notes

- Slack requests are verified via `SLACK_SIGNING_SECRET` using HMAC-SHA256 (`X-Slack-Signature` + `X-Slack-Request-Timestamp`). Slack retries (`X-Slack-Retry-Num` set) are acked without re-processing to avoid duplicate Paperclip issues.
- Paperclip webhooks must include `X-Paperclip-Signature: sha256=<hex hmac>` computed over the raw request body using `PAPERCLIP_WEBHOOK_SECRET`. Unsigned requests are rejected with 401.
- Replies are pinned to the `slack_channel` / `slack_thread_ts` recorded in the issue's `metadata`; the agent-supplied `slack_reply.channel`/`thread_ts` is only used as a fallback.
- Webhooks are deduped by `issue_id` (in-memory) to absorb Paperclip retries.
- The Paperclip create-issue endpoint path (`/api/issues`) is a placeholder — adjust it to match your Paperclip deployment.

## License

MIT — see [LICENSE.md](LICENSE.md).
