# Deepsleep-0B

Deepsleep-0B is a small Go server that exposes OpenAI-compatible and Claude-compatible chat endpoints for the `deepsleep` and `deepsleep-0b` models.

Try the hosted model here:

https://deepsleep.isclaude.com

## Endpoints

- OpenAI: `https://deepsleep.isclaude.com/v1/chat/completions`
- Claude: `https://deepsleep.isclaude.com/v1/messages`
- Models: `https://deepsleep.isclaude.com/v1/models`

Any model name and API token are accepted.

## Run Locally

```bash
go run ./cmd/deepsleep
```

Open:

```text
http://localhost:8080
```

## Docker Compose

```bash
docker compose up --build
```

The default domain shown in the UI is configured in `config.json`.
