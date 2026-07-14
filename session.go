package soulbits

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// ConnectSession acquires a session lock and assigns a warm-pool HL container.
// An optional version can be specified to request a specific HL image version.
func (c *Client) ConnectSession(ctx context.Context, version *string) (*gen.SessionConnectResponse, error) {
	body := gen.PostV1SessionConnectJSONRequestBody(gen.PostV1SessionConnectJSONBody{Version: version})
	resp, err := c.cloudGen.PostV1SessionConnectWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("connect session request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// DisconnectSession transitions the session to a 5-minute grace period.
func (c *Client) DisconnectSession(ctx context.Context, sessionID *string, reason *string) (*gen.SessionDisconnectResponse, error) {
	body := gen.PostV1SessionDisconnectJSONRequestBody(gen.PostV1SessionDisconnectJSONBody{
		Reason: reason,
	})
	if sessionID != nil {
		uid, err := uuid.Parse(*sessionID)
		if err != nil {
			return nil, fmt.Errorf("invalid session id %q: %w", *sessionID, err)
		}
		// We need to rebuild the body with the session_id set.
		body = gen.PostV1SessionDisconnectJSONRequestBody(gen.PostV1SessionDisconnectJSONBody{
			SessionId: &uid,
			Reason:    reason,
		})
	}
	resp, err := c.cloudGen.PostV1SessionDisconnectWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("disconnect session request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ReportConnected reports that the WebSocket upgrade has completed.
// Called by the conduct proxy; typically not called by client code directly.
func (c *Client) ReportConnected(ctx context.Context) (*gen.SessionConnectedResponse, error) {
	resp, err := c.cloudGen.PostV1SessionConnectedWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("report connected request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// Versions lists available Harmony Link image versions.
func (c *Client) Versions(ctx context.Context) (*gen.VersionsResponse, error) {
	resp, err := c.cloudGen.GetV1VersionsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("versions request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}
