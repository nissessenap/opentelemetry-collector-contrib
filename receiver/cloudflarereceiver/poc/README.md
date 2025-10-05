# Cloudflare GraphQL API - Firewall Events PoC

This is a simple proof-of-concept to demonstrate fetching firewall/WAF analytics from Cloudflare's GraphQL API.

## Purpose

Verify that we can:
1. Authenticate with Cloudflare's GraphQL API
2. Query firewall events using the `firewallEventsAdaptiveGroups` dataset
3. Get aggregated metrics by action, source, and country (low cardinality)
4. Parse the response into usable data structures

## Prerequisites

1. **Cloudflare Account** with at least one zone
2. **API Token** with the correct permissions:
   - Go to: https://dash.cloudflare.com/profile/api-tokens
   - Click "Create Token"
   - **Option 1**: Use "Read analytics" template (recommended)
   - **Option 2**: Create custom token with these permissions:
     - **Account** → **Analytics** → **Read** ✅ (Required for firewall events)
     - **Zone** → **Analytics** → **Read** (for zone-level queries)
3. **Zone ID** - Found in your Cloudflare dashboard under the zone overview

## Setup

### 1. Get your Zone ID

Visit your Cloudflare dashboard and copy the Zone ID from the right sidebar of any zone.

### 2. Set environment variables

```bash
export CF_API_TOKEN="your-api-token-here"
export CF_ZONE_ID="your-zone-id-here"
```

### 3. Run the PoC

```bash
cd receiver/cloudflarereceiver/poc
go run .
```

## What It Does

The PoC:
1. Creates a simple GraphQL client using only stdlib (`net/http`)
2. Queries firewall events for the **last hour**
3. Fetches aggregated data by:
   - `action` (allow, block, challenge, etc.)
   - `source` (waf, firewallRules, rateLimit, etc.)
   - `clientCountryName` (country)
4. Prints:
   - Raw JSON response
   - Summary by action
   - Summary by source
   - Top 10 countries

## Expected Output

```
Fetching firewall events for zone abc123...
Time range: 2025-10-05T10:00:00Z to 2025-10-05T11:00:00Z

Found 15 aggregated firewall event groups

Firewall Events (aggregated by action/source/country):
[
  {
    "count": 1234,
    "dimensions": {
      "action": "block",
      "source": "waf",
      "clientCountryName": "United States"
    }
  },
  ...
]

=== Summary by Action ===
block: 5678
allow: 1234
challenge: 890

=== Summary by Source ===
waf: 4567
firewallRules: 2345
rateLimit: 890

=== Top 10 Countries ===
United States: 3456
China: 1234
Germany: 890
...

✅ PoC completed successfully!
This demonstrates that we can fetch firewall analytics via GraphQL API
```

## No Events?

If you see "Found 0 aggregated firewall event groups", it means:
1. Your zone had no firewall events in the last hour
2. Try increasing the time range in `main.go` (change `-1 * time.Hour` to `-24 * time.Hour`)
3. Or generate some test traffic that triggers WAF rules

## Code Structure

- **`graphql_client.go`**: Simple GraphQL client
  - `GraphQLClient`: HTTP client wrapper
  - `Query()`: Generic GraphQL query executor
  - `GetFirewallEvents()`: Specific query for firewall events
  - `FirewallEventsResponse`: Response data structure

- **`main.go`**: Example usage
  - Reads config from environment
  - Calls GraphQL API
  - Formats and prints results

## Key Learnings

### ✅ Pros
- GraphQL API is straightforward to use
- No third-party library needed (just `net/http`)
- Aggregated queries provide low-cardinality metrics
- Response is well-structured and easy to parse

### ⚠️ Considerations
- GraphQL queries need to be crafted carefully
- Error handling requires checking both HTTP status and GraphQL errors
- Time format is RFC3339
- Pagination might be needed for large result sets (limit: 100 in this PoC)

## Next Steps

If this PoC succeeds, we can:
1. Integrate this GraphQL client into the receiver implementation
2. Add it alongside the REST client for zone analytics
3. Convert firewall events to OTEL metrics
4. Add proper error handling, retries, and rate limiting

## Troubleshooting

### Authentication Error
```
Error fetching firewall events: unexpected status code 403
```
**Solution**: Check your API token has `Analytics:Read` permission

### Invalid Zone ID
```
Error fetching firewall events: graphql errors: [...]
```
**Solution**: Verify your Zone ID is correct

### Network Error
```
Error fetching firewall events: execute request: ...
```
**Solution**: Check your internet connection and firewall settings

## References

- [Cloudflare GraphQL Analytics API Docs](https://developers.cloudflare.com/analytics/graphql-api/)
- [Querying Firewall Events Tutorial](https://developers.cloudflare.com/analytics/graphql-api/tutorials/querying-firewall-events/)
- [API Token Setup](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)
