package soulbits

import (
	"context"
	"fmt"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// ListModels returns all active models from the catalog.
// Supports optional filters by model type and modalities.
func (c *Client) ListModels(ctx context.Context, filter *ModelFilter) ([]gen.ModelEntry, error) {
	var params *gen.GetV1ModelsParams
	if filter != nil {
		mt := gen.ModelType(filter.ModelType)
		im := gen.InputModalities(filter.InputModalities)
		om := gen.OutputModalities(filter.OutputModalities)
		params = &gen.GetV1ModelsParams{
			ModelType:        &mt,
			InputModalities:  &im,
			OutputModalities: &om,
		}
		// If ModelType is empty, nil it so the query param is omitted.
		if filter.ModelType == "" {
			params.ModelType = nil
		}
		if len(filter.InputModalities) == 0 {
			params.InputModalities = nil
		}
		if len(filter.OutputModalities) == 0 {
			params.OutputModalities = nil
		}
	}

	resp, err := c.inferGen.GetV1ModelsWithResponse(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list models request: %w", err)
	}
	models, err := checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
	if err != nil {
		return nil, err
	}
	return *models, nil
}

// ModelFilter filters the model catalog.
type ModelFilter struct {
	// ModelType filters by exact model type.
	// Valid values: llm, tts, stt, vad, embeddings, rerank, voice_embed,
	// audio_conversion, voice_conversion.
	ModelType gen.ModelEntryModelType

	// InputModalities is a superset filter — model must accept ALL listed
	// input modalities (e.g. ["text", "image"]).
	InputModalities []string

	// OutputModalities is a superset filter — model must emit ALL listed
	// output modalities.
	OutputModalities []string
}

// Validate returns an error if the filter has invalid values.
func (f *ModelFilter) Validate() error {
	validTypes := map[gen.ModelEntryModelType]bool{
		gen.ModelEntryModelTypeLlm:             true,
		gen.ModelEntryModelTypeTts:             true,
		gen.ModelEntryModelTypeStt:             true,
		gen.ModelEntryModelTypeVad:             true,
		gen.ModelEntryModelTypeImage:           true,
		gen.ModelEntryModelTypeEmbeddings:      true,
		gen.ModelEntryModelTypeRerank:          true,
		gen.ModelEntryModelTypeVoiceEmbed:      true,
		gen.ModelEntryModelTypeAudioConversion: true,
		gen.ModelEntryModelTypeVoiceConversion: true,
	}
	if f.ModelType != "" && !validTypes[f.ModelType] {
		return fmt.Errorf("invalid model type: %q", f.ModelType)
	}
	return nil
}
