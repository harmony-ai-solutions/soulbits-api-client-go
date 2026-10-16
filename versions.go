package soulbits

import (
	"context"
	"fmt"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// Versions lists available Harmony Link image versions.
//
// Retained after the session-hosting retirement (phase 6-1): the
// `/v1/versions` endpoint is not a session-plane route and remains part of the
// contract. The rest of the former session facade (connect/disconnect/
// connected/data-purge) was removed with the `/v1/session/*` paths.
func (c *Client) Versions(ctx context.Context) (*gen.VersionsResponse, error) {
	resp, err := c.cloudGen.GetV1VersionsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("versions request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}
