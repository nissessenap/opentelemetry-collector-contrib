// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	// Get configuration from environment variables
	apiToken := os.Getenv("CF_API_TOKEN")
	zoneID := os.Getenv("CF_ZONE_ID")
	debug := os.Getenv("DEBUG") == "true"

	if apiToken == "" {
		fmt.Println("Error: CF_API_TOKEN environment variable is required")
		fmt.Println("\nTo create an API token:")
		fmt.Println("1. Go to https://dash.cloudflare.com/profile/api-tokens")
		fmt.Println("2. Create Token -> Use 'Read analytics' template")
		fmt.Println("3. Or create custom token with 'Account.Analytics:Read' permission")
		os.Exit(1)
	}

	if zoneID == "" {
		fmt.Println("Error: CF_ZONE_ID environment variable is required")
		fmt.Println("\nTo find your Zone ID:")
		fmt.Println("1. Go to your Cloudflare dashboard")
		fmt.Println("2. Select a domain/zone")
		fmt.Println("3. Look in the right sidebar for 'Zone ID'")
		os.Exit(1)
	}

	fmt.Println("=== Cloudflare GraphQL Firewall Analytics PoC ===\n")

	// Create GraphQL client
	client := NewGraphQLClient(apiToken)

	// Query firewall events for the last hour
	until := time.Now().UTC()
	since := until.Add(-10 * time.Hour)

	fmt.Printf("Zone ID: %s\n", zoneID)
	fmt.Printf("Time range: %s to %s\n", since.Format(time.RFC3339), until.Format(time.RFC3339))
	fmt.Printf("Debug mode: %v\n", debug)

	ctx := context.Background()
	result, err := client.GetFirewallEvents(ctx, zoneID, since, until, debug)
	if err != nil {
		fmt.Printf("\n❌ Error fetching firewall events: %v\n", err)
		fmt.Println("\nTroubleshooting:")
		fmt.Println("1. Verify your API token has 'Account.Analytics:Read' permission")
		fmt.Println("2. Check that your Zone ID is correct")
		fmt.Println("3. Try running with DEBUG=true for more details")
		fmt.Println("4. Example: DEBUG=true CF_API_TOKEN=xxx CF_ZONE_ID=yyy go run .")
		os.Exit(1)
	}

	// Print results
	if len(result.Viewer.Zones) == 0 {
		fmt.Println("No zones found in response")
		return
	}

	zone := result.Viewer.Zones[0]
	events := zone.FirewallEventsAdaptiveGroups

	fmt.Printf("Found %d aggregated firewall event groups\n\n", len(events))

	// Pretty print the results
	prettyJSON, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Firewall Events (aggregated by action/source/country):")
	fmt.Println(string(prettyJSON))

	// Print summary by action
	fmt.Println("\n=== Summary by Action ===")
	actionCounts := make(map[string]int64)
	for _, event := range events {
		actionCounts[event.Dimensions.Action] += event.Count
	}
	for action, count := range actionCounts {
		fmt.Printf("%s: %d\n", action, count)
	}

	// Print summary by source
	fmt.Println("\n=== Summary by Source ===")
	sourceCounts := make(map[string]int64)
	for _, event := range events {
		sourceCounts[event.Dimensions.Source] += event.Count
	}
	for source, count := range sourceCounts {
		fmt.Printf("%s: %d\n", source, count)
	}

	// Print top countries
	fmt.Println("\n=== Top 10 Countries ===")
	countryCounts := make(map[string]int64)
	for _, event := range events {
		countryCounts[event.Dimensions.ClientCountryName] += event.Count
	}

	// Sort and print top 10
	type countryCount struct {
		country string
		count   int64
	}
	var countries []countryCount
	for country, count := range countryCounts {
		countries = append(countries, countryCount{country, count})
	}

	// Simple bubble sort (good enough for small dataset)
	for i := 0; i < len(countries); i++ {
		for j := i + 1; j < len(countries); j++ {
			if countries[j].count > countries[i].count {
				countries[i], countries[j] = countries[j], countries[i]
			}
		}
	}

	max := len(countries)
	if max > 10 {
		max = 10
	}
	for i := 0; i < max; i++ {
		fmt.Printf("%s: %d\n", countries[i].country, countries[i].count)
	}

	fmt.Println("\n✅ PoC completed successfully!")
	fmt.Println("This demonstrates that we can fetch firewall analytics via GraphQL API")
}
