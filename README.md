# Soulbits API Client — Go

First-party Go client for the [Soulbits Cloud Platform](https://soulbits.app) REST API.

## Installation

```bash
go get github.com/harmony-ai-solutions/soulbits-api-client-go
```

## Quick Start

### API Key Auth (server-side)

```go
package main

import (
    "context"
    "fmt"
    "log"

    soulbits "github.com/harmony-ai-solutions/soulbits-api-client-go"
)

func main() {
    client, err := soulbits.New(&soulbits.Config{
        APIKey: "sb_cloud_your_api_key_here",
    })
    if err != nil {
        log.Fatal(err)
    }

    models, err := client.ListModels(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Available models: %d\n", len(models))
}
```

### PASETO Auth (user-facing)

```go
client, err := soulbits.New(&soulbits.Config{
    PasetoToken:  "v4.local.xxx...",
    RefreshToken: "opaque_refresh_token",
})
```

### Multi-Host Architecture

The client automatically routes requests to the correct host:

| Host | Operations |
|------|-----------|
| `cloud.soulbits.app` | Auth, account, API keys, session, subscription |
| `api.soulbits.app` | Chat, embeddings, rerank, TTS, STT, VAD, voice/audio conversion |

## Usage Examples

### Chat (OpenAI-compatible, non-streaming)

```go
resp, err := client.Chat(ctx, &gen.ChatCompletionRequest{
    Model: "qwen-35-9b",
    Messages: []gen.ChatMessage{
        {Role: "user", Content: "Hello!"},
    },
})
```

### Text Embeddings

```go
resp, err := client.Embed(ctx, &gen.EmbeddingRequest{
    Model: "qwen3-embed-4b",
    Input: "Hello world",
})
```

### TTS (Harmony Speech Engine)

```go
audioBytes, _ := base64.StdEncoding.DecodeString(voiceSample)
resp, err := client.TTS(ctx, &gen.TTSRequest{
    Model: "kitten-tts-mini",
    Input: "Hello from Soulbits",
    Mode:  gen.SingleSpeakerTts,
})
```

### Authentication

```go
// Login
token, err := client.Login(ctx, "user@example.com", "password")

// Refresh (PASETO only)
newToken, err := client.Refresh(ctx)

// Register
msg, err := client.Register(ctx, "user@example.com", "password", nil)
```

## Regenerating from the OpenAPI Spec

```bash
# Install the generator (one time)
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

# Regenerate
go generate ./...
```

The `openapi.yaml` is the source of truth. Generated code lives in `gen/client.go` and is committed to the repo.

## Development

```bash
go generate ./...   # regenerate from spec
go build ./...      # compile
go test ./...       # run tests (no network required)
go vet ./...        # static analysis
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
