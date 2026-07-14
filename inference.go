package soulbits

import (
	"context"
	"fmt"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// ---------------------------------------------------------------------------
// Chat
// ---------------------------------------------------------------------------

// Chat sends a chat completion request to the Inference API.
// The response is always non-streaming (stream:true is silently ignored by the
// gateway).
func (c *Client) Chat(ctx context.Context, req *gen.ChatCompletionRequest) (*gen.ChatCompletionResponse, error) {
	resp, err := c.inferGen.PostV1ChatCompletionsWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Embeddings
// ---------------------------------------------------------------------------

// Embed creates text embeddings using the Inference API.
func (c *Client) Embed(ctx context.Context, req *gen.EmbeddingRequest) (*gen.EmbeddingResponse, error) {
	resp, err := c.inferGen.PostV1EmbeddingsWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Rerank
// ---------------------------------------------------------------------------

// Rerank ranks documents by their relevance to a query.
func (c *Client) Rerank(ctx context.Context, req *gen.RerankRequest) (*gen.RerankResponse, error) {
	resp, err := c.inferGen.PostV1RerankWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// TTS
// ---------------------------------------------------------------------------

// TTS synthesises speech from text using the Harmony Speech Engine.
func (c *Client) TTS(ctx context.Context, req *gen.TTSRequest) (*gen.AudioDataResponse, error) {
	resp, err := c.inferGen.PostV1AudioSpeechWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("tts request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// STT
// ---------------------------------------------------------------------------

// STT transcribes audio to text using the Harmony Speech Engine.
func (c *Client) STT(ctx context.Context, req *gen.STTRequest) (*gen.STTResponse, error) {
	resp, err := c.inferGen.PostV1AudioTranscriptionsWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("stt request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// VAD
// ---------------------------------------------------------------------------

// VAD performs voice activity detection on audio.
func (c *Client) VAD(ctx context.Context, req *gen.VADRequest) (*gen.VADResponse, error) {
	resp, err := c.inferGen.PostV1AudioVadWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("vad request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Speaker Embedding
// ---------------------------------------------------------------------------

// SpeakerEmbed creates a speaker embedding from an audio sample.
// The returned AudioDataResponse.Data contains the base64-encoded embedding.
func (c *Client) SpeakerEmbed(ctx context.Context, req *gen.SpeakerEmbedRequest) (*gen.AudioDataResponse, error) {
	resp, err := c.inferGen.PostV1EmbedSpeakerWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("speaker embed request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Audio Conversion
// ---------------------------------------------------------------------------

// AudioConvert converts/enhances audio (e.g. denoising via VoiceFixer).
func (c *Client) AudioConvert(ctx context.Context, req *gen.AudioConversionRequest) (*gen.AudioDataResponse, error) {
	resp, err := c.inferGen.PostV1AudioConvertWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("audio convert request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Voice Conversion
// ---------------------------------------------------------------------------

// VoiceConvert converts the voice in source audio to sound like a target speaker.
func (c *Client) VoiceConvert(ctx context.Context, req *gen.VoiceConversionRequest) (*gen.AudioDataResponse, error) {
	resp, err := c.inferGen.PostV1VoiceConvertWithResponse(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("voice convert request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Image Generation (deprecated)
// ---------------------------------------------------------------------------

// ImageGen generates images. Deprecated — no worker is currently deployed.
// Attempts will likely result in a 503 or gateway timeout.
func (c *Client) ImageGen(ctx context.Context, req *gen.ImageGenerationRequest) error {
	resp, err := c.inferGen.PostV1ImagesGenerationsWithResponse(ctx, *req)
	if err != nil {
		return fmt.Errorf("image gen request: %w", err)
	}
	return responseError(resp.Body, resp.HTTPResponse)
}
