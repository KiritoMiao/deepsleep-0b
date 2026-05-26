# Deepsleep-0B

Deepsleep-0B is a small Go server that exposes OpenAI-compatible and Claude-compatible chat endpoints for the `deepsleep`, `deepsleep-0b`, and `deepsleep-think` models.

Try the hosted model here:

https://deepsleep.isclaude.com

## Endpoints

- OpenAI: `https://deepsleep.isclaude.com/v1/chat/completions`
- Claude: `https://deepsleep.isclaude.com/v1/messages`
- Models: `https://deepsleep.isclaude.com/v1/models`

Any model name and API token are accepted.

Use `deepsleep-think` with `stream: true` to get a delayed fake thinking stream followed by a short sleepy answer such as `sleeping...` or `dozing...`.

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
