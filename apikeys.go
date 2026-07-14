package soulbits

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// CreateAPIKey creates a new API key. The full key is returned only once.
// API keys CANNOT create/list/revoke other API keys (PASETO auth required).
func (c *Client) CreateAPIKey(ctx context.Context, name *string) (*gen.APIKeyCreated, error) {
	body := gen.PostV1AuthApiKeysJSONRequestBody(gen.PostV1AuthApiKeysJSONBody{Name: name})
	resp, err := c.cloudGen.PostV1AuthApiKeysWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("create api key request: %w", err)
	}
	return checkOK(resp.JSON201, resp.Body, resp.HTTPResponse)
}

// ListAPIKeys lists all API keys for the current user.
// The raw key value is never returned — only the prefix for identification.
func (c *Client) ListAPIKeys(ctx context.Context) ([]gen.APIKeyListItem, error) {
	resp, err := c.cloudGen.GetV1AuthApiKeysWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api keys request: %w", err)
	}
	keys, err := checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
	if err != nil {
		return nil, err
	}
	return *keys, nil
}

// RevokeAPIKey revokes an API key by ID. Idempotent.
func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("api key id is required")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid api key id %q: %w", id, err)
	}
	resp, err := c.cloudGen.DeleteV1AuthApiKeysIdWithResponse(ctx, uid)
	if err != nil {
		return fmt.Errorf("revoke api key request: %w", err)
	}
	return responseError(resp.Body, resp.HTTPResponse)
}
