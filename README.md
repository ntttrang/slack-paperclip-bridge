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
| `SLACK_SIGNING_SECRET` | no | Slack signing secret (currently unused; signature verification is disabled in demo). |
| `PAPERCLIP_BASE_URL` | yes | Base URL of the Paperclip API. |
| `PAPERCLIP_API_KEY` | yes | Bearer token for the Paperclip API. |
| `INTAKE_AGENT_ID` | yes | Paperclip agent ID that incoming Slack issues are assigned to. |
| `LISTEN_ADDR` | no | HTTP listen address. Defaults to `:8080`. |

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

- Slack request signature verification is disabled in this demo (`OptionNoVerifyToken`). Enable it before running in production.
- The Paperclip create-issue endpoint path (`/api/issues`) is a placeholder — adjust it to match your Paperclip deployment.

## License

MIT — see [LICENSE.md](LICENSE.md).
