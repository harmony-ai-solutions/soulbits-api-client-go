package soulbits

import (
	"context"
	"fmt"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// ListTiers lists all subscription tiers. Public endpoint — no auth required.
func (c *Client) ListTiers(ctx context.Context) ([]gen.Tier, error) {
	resp, err := c.cloudGen.GetV1SubscriptionTiersWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tiers request: %w", err)
	}
	tiers, err := checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
	if err != nil {
		return nil, err
	}
	return *tiers, nil
}

// GetMySubscription returns the current user's subscription.
// Defaults to "free" if no subscription exists.
func (c *Client) GetMySubscription(ctx context.Context) (*gen.SubscriptionMe, error) {
	resp, err := c.cloudGen.GetV1SubscriptionMeWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("get subscription request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}
