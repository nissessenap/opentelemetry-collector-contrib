// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const cloudflareGraphQLEndpoint = "https://api.cloudflare.com/client/v4/graphql"

// GraphQLClient is a simple client for Cloudflare's GraphQL API
type GraphQLClient struct {
	httpClient *http.Client
	apiToken   string
}

// NewGraphQLClient creates a new GraphQL client
func NewGraphQLClient(apiToken string) *GraphQLClient {
	return &GraphQLClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiToken: apiToken,
	}
}

// GraphQLRequest represents a GraphQL query request
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message string   `json:"message"`
	Path    []string `json:"path,omitempty"`
}

// Query executes a GraphQL query
func (c *GraphQLClient) Query(ctx context.Context, query string, vars map[string]interface{}, debug bool) (*GraphQLResponse, error) {
	req := GraphQLRequest{
		Query:     query,
		Variables: vars,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if debug {
		fmt.Println("\n=== DEBUG: Request ===")
		fmt.Printf("Endpoint: %s\n", cloudflareGraphQLEndpoint)
		tokenLen := len(c.apiToken)
		if tokenLen > 10 {
			fmt.Printf("API Token: %s...%s (length: %d)\n", c.apiToken[:5], c.apiToken[tokenLen-5:], tokenLen)
		} else {
			fmt.Printf("API Token: (too short - length: %d)\n", tokenLen)
		}
		fmt.Printf("Variables: %+v\n", vars)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", cloudflareGraphQLEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if debug {
		fmt.Println("\n=== DEBUG: Response ===")
		fmt.Printf("Status Code: %d\n", resp.StatusCode)
		fmt.Printf("Response Body: %s\n", string(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result GraphQLResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql errors: %v", result.Errors)
	}

	return &result, nil
}

// FirewallEventsResponse represents the response from firewall events query
type FirewallEventsResponse struct {
	Viewer struct {
		Zones []struct {
			FirewallEventsAdaptiveGroups []struct {
				Count      int64 `json:"count"`
				Dimensions struct {
					Action            string `json:"action"`
					Source            string `json:"source"`
					ClientCountryName string `json:"clientCountryName"`
				} `json:"dimensions"`
			} `json:"firewallEventsAdaptiveGroups"`
		} `json:"zones"`
	} `json:"viewer"`
}

// GetFirewallEvents fetches aggregated firewall events for a zone
func (c *GraphQLClient) GetFirewallEvents(ctx context.Context, zoneID string, since, until time.Time, debug bool) (*FirewallEventsResponse, error) {
	query := `
		query FirewallEventsByAction($zoneTag: String!, $since: Time!, $until: Time!) {
			viewer {
				zones(filter: { zoneTag: $zoneTag }) {
					firewallEventsAdaptiveGroups(
						filter: {
							datetime_geq: $since,
							datetime_leq: $until
						},
						limit: 100,
						orderBy: [count_DESC]
					) {
						count
						dimensions {
							action
							source
							clientCountryName
						}
					}
				}
			}
		}
	`

	vars := map[string]interface{}{
		"zoneTag": zoneID,
		"since":   since.Format(time.RFC3339),
		"until":   until.Format(time.RFC3339),
	}

	resp, err := c.Query(ctx, query, vars, debug)
	if err != nil {
		return nil, err
	}

	var result FirewallEventsResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal firewall events: %w", err)
	}

	return &result, nil
}
