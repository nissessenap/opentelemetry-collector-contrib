# Cloudflare GraphQL Analytics API Research

## Executive Summary

**Finding**: Firewall/WAF analytics in Cloudflare are **only available via GraphQL API**, not REST API.

**Recommendation**: Include basic firewall metrics in initial implementation - complexity is manageable.

**PoC Status**: ✅ **COMPLETED & VALIDATED**

- Successfully queried `firewallEventsAdaptiveGroups` via GraphQL
- Tested with Zone.Analytics:Read permission (account-level not required for zone queries)
- Confirmed aggregation approach works with action, source, and country dimensions
- Example output: `{action: "block", source: "firewallManaged", country: "BR", count: 1}`
- Decision: **Country dimension opt-in** (`include_country_dimension: false` by default)
  - Balances observability needs with infrastructure capacity
  - Default: ~100 time series per zone (manageable for standard Prometheus)
  - Opt-in: ~20,000 time series per zone (requires Thanos/Mimir)

**Time Window Strategy**:

- Default collection_interval: 60 seconds
- Each scrape queries: `datetime_geq: lastScrapeTime, datetime_leq: now()`
- Store last scrape timestamp to avoid gaps or overlaps
- Example: If scraped at 10:00:00, next scrape at 10:01:00 queries [10:00:00 → 10:01:00]
- Handles clock drift and delays gracefully

---

## GraphQL Support in Cloudflare Go SDK

### Official SDK (cloudflare-go v6)

- ❌ **No built-in GraphQL support**
- ✅ Provides REST API access only
- Used for: Zone listing, account info, DNS records, etc.

### Third-Party Option

- **Library**: `github.com/zeet-dev/cloudflare-graphql-go`
- **Maturity**: Early stage (3 commits)
- **Features**:
  - Provides GraphQL client wrapper
  - Has auto-generated types from GraphQL schema
  - Only example: `GetZoneAnalyticsByDayQuery` (zone analytics)
  - ❌ No pre-built firewall event queries
- **Verdict**: Not production-ready, but shows the pattern

---

## GraphQL API Details

### Endpoint

```
POST https://api.cloudflare.com/client/v4/graphql
```

### Authentication

```
Authorization: Bearer <API_TOKEN>
Content-Type: application/json
```

### Request Structure

```json
{
  "query": "{ viewer { zones(filter: { zoneTag: $zoneTag }) { firewallEventsAdaptive(...) { ... } } } }",
  "variables": {
    "zoneTag": "zone-id",
    "filter": {
      "datetime_geq": "2022-07-24T11:00:00Z",
      "datetime_leq": "2022-07-24T12:00:00Z"
    }
  }
}
```

---

## Firewall Events Data

### Dataset: `firewallEventsAdaptive`

Available for individual events (for aggregations, use `firewallEventsAdaptiveGroups`)

### Available Fields (Low Cardinality)

- ✅ `action` - Action taken (allow, block, challenge, jschallenge, log, connectionClose, etc.)
- ✅ `source` - Source of event (waf, firewallRules, rateLimit, securityLevel, etc.)
- ✅ `datetime` - Timestamp of event
- ✅ `clientCountryName` - Client country
- ✅ `clientAsn` - Client ASN

### High Cardinality Fields (Use with Caution)

- ⚠️ `ruleId` - Specific rule that triggered (hundreds/thousands of unique values)
- ⚠️ `clientIP` - Client IP address
- ⚠️ `clientRequestPath` - Request path
- ⚠️ `userAgent` - User agent string
- ⚠️ `rayName` - Cloudflare Ray ID

---

## Implementation Complexity Analysis

### Option 1: Use Third-Party Library

**Pros**:

- Saves some boilerplate code
- Has type generation from schema

**Cons**:

- Immature library (3 commits)
- No firewall queries pre-built
- Extra dependency to maintain
- Still need to write custom queries

**Complexity**: Medium

### Option 2: Direct HTTP Client (Recommended)

**Pros**:

- Full control over queries
- No extra dependencies beyond stdlib
- Simple to understand and debug
- Can use `net/http` package

**Cons**:

- More boilerplate code
- Need to define our own types

**Complexity**: Low-Medium

### Implementation Estimate

- GraphQL client wrapper: 2-3 hours
- Firewall events query: 2-3 hours
- Type definitions: 1-2 hours
- Tests: 2-3 hours
- **Total: 8-11 hours**

---

## Recommended Metrics ✅ FINAL

### Zone-Level Firewall

```
cloudflare.zone.firewall.requests
  Resources: zone_id, zone_name
  Dimensions (always): action, source
  Dimensions (opt-in): country
```

### Dimensions - Always Included

- **action**: allow, block, challenge, jschallenge, log, connectionClose (~10 values) ✅
  - **Use case**: Alert on spike in blocks (attack), challenges (bots)
  - **Essential**: Core operational metrics
- **source**: waf, firewallManaged, firewallRules, rateLimit, securityLevel (~10 values) ✅
  - **Use case**: Identify attack vector, rule effectiveness
  - **Essential**: Understanding what security layer is active

### Dimensions - Opt-In via Config

- **country**: ISO country codes (~200 values) ⚙️ `include_country_dimension: true`
  - **Use case**: Geographic threat analysis, geofencing, compliance
  - **Trade-off**: 200x cardinality increase
  - **When to enable**:
    - Geographic security requirements
    - Need compatibility with lablabs/cloudflare-exporter
    - Have proper metrics backend (Thanos/Mimir)

### Dimensions - Always Excluded

- **rule_id**: Specific firewall rules (thousands of values) ❌
  - **Reason**: Extremely high cardinality, forensics not operational monitoring
- **clientIP, clientRequestPath, userAgent**: Individual identifiers ❌
  - **Reason**: Log analysis domain

**Cardinality Summary**:

| Configuration | Per Zone | 100 Zones | Backend |
|--------------|----------|-----------|---------|
| **Default** (action + source) | ~100 | ~10,000 | Standard Prometheus |
| **With country** | ~20,000 | ~2,000,000 | Prometheus + Thanos/Mimir |

**What You Alert On (Default)**:

- `rate(firewall.requests{action="block"}[5m]) > threshold` → Attack detected
- `rate(firewall.requests{action="challenge"}[5m]) > threshold` → Bot spike
- `firewall.requests{source="rateLimit"}` → Rate limiting triggered
- Sudden changes in action/source distribution → New attack pattern

**Additional Alerts (If Country Enabled)**:

- `rate(firewall.requests{action="block",country="XX"}[5m]) > threshold` → Targeted attack from country

---

## Sample GraphQL Query for Aggregated Firewall Events

```graphql
query FirewallEventsByAction($zoneTag: String!, $since: String!, $until: String!) {
  viewer {
    zones(filter: { zoneTag: $zoneTag }) {
      firewallEventsAdaptiveGroups(
        filter: {
          datetime_geq: $since,
          datetime_leq: $until
        },
        limit: 1000,
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
```

---

## Sample Go Implementation Sketch

```go
package client

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

const cloudflareGraphQLEndpoint = "https://api.cloudflare.com/client/v4/graphql"

type GraphQLClient struct {
    httpClient *http.Client
    apiToken   string
}

type GraphQLRequest struct {
    Query     string                 `json:"query"`
    Variables map[string]interface{} `json:"variables"`
}

type GraphQLResponse struct {
    Data   json.RawMessage `json:"data"`
    Errors []GraphQLError  `json:"errors,omitempty"`
}

type GraphQLError struct {
    Message string `json:"message"`
}

func (c *GraphQLClient) Query(ctx context.Context, query string, vars map[string]interface{}) (*GraphQLResponse, error) {
    req := GraphQLRequest{
        Query:     query,
        Variables: vars,
    }

    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
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

    var result GraphQLResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    if len(result.Errors) > 0 {
        return nil, fmt.Errorf("graphql errors: %v", result.Errors)
    }

    return &result, nil
}

func (c *GraphQLClient) GetFirewallEvents(ctx context.Context, zoneID, since, until string) (*FirewallEventsResponse, error) {
    query := `
        query FirewallEventsByAction($zoneTag: String!, $since: String!, $until: String!) {
            viewer {
                zones(filter: { zoneTag: $zoneTag }) {
                    firewallEventsAdaptiveGroups(
                        filter: {
                            datetime_geq: $since,
                            datetime_leq: $until
                        },
                        limit: 1000,
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
        "since":   since,
        "until":   until,
    }

    resp, err := c.Query(ctx, query, vars)
    if err != nil {
        return nil, err
    }

    var result FirewallEventsResponse
    if err := json.Unmarshal(resp.Data, &result); err != nil {
        return nil, fmt.Errorf("unmarshal firewall events: %w", err)
    }

    return &result, nil
}

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
```

---

## Recommendation for Implementation Plan

### Include in Initial Implementation ✅

**Rationale**:

1. Implementation complexity is **manageable** (8-11 hours)
2. Direct HTTP approach is **simple** and has no extra dependencies
3. Low cardinality metrics are **safe** for metrics backends
4. Firewall metrics provide **high value** for security monitoring
5. Pattern can be **reused** for future GraphQL queries (Workers, R2, etc.)

### Staged Approach

**Stage 2: Client Implementation**

- Add GraphQL client alongside REST client
- Implement `GetFirewallEventsByAction()`
- Implement `GetFirewallEventsBySource()`

**Stage 3: Scraper Implementation**

- Add firewall events scraping
- Aggregate by action and source
- Convert to OTEL metrics

**Stage 4: Metrics Metadata**

- Define firewall metrics in metadata.yaml
- Low cardinality dimensions only

---

## Open Questions Resolved

1. ✅ **GraphQL Support**: Direct HTTP client is simple and sufficient
2. ✅ **Cardinality**: Use aggregated queries with action/source dimensions only
3. ✅ **Complexity**: Manageable addition to initial implementation (~8-11 hours)

---

## References

- [Cloudflare GraphQL Analytics API](https://developers.cloudflare.com/analytics/graphql-api/)
- [Querying Firewall Events with GraphQL](https://developers.cloudflare.com/analytics/graphql-api/tutorials/querying-firewall-events/)
- [Execute GraphQL Query](https://developers.cloudflare.com/analytics/graphql-api/getting-started/execute-graphql-query/)
- [zeet-dev/cloudflare-graphql-go](https://github.com/zeet-dev/cloudflare-graphql-go)
