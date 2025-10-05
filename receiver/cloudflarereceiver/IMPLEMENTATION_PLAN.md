# Cloudflare Receiver - Metrics Implementation Plan

## Overview

Extend the existing Cloudflare receiver to support metrics collection from Cloudflare Analytics API. The receiver currently supports logs via LogPush; this implementation adds metrics collection for zones and WAF analytics.

## Context

- **Existing Component**: Cloudflare receiver (logs only, alpha stability)
- **New Capability**: Add metrics scraping from Cloudflare Analytics API
- **SDK**: Use `github.com/cloudflare/cloudflare-go/v6` for API interaction
- **Architecture Pattern**: Follow scraperhelper pattern (similar to azuremonitor receiver)
- **Stability Target**: Development → Alpha

## Goals

1. Add metrics collection capability to existing cloudflarereceiver
2. Support Zone analytics (bandwidth, requests, threats, pageviews)
3. Support Firewall/WAF analytics (by action and source, country opt-in)
4. Use Cloudflare Go SDK v6 for REST API interactions
5. Use GraphQL API directly (via HTTP client) for firewall analytics
6. Auto-discover all zones in account by default with zone filtering support
7. Design for extensibility (easy to add account-level, Workers, R2, Logpush metrics later)
8. Maintain separation from existing logs functionality
9. Default collection_interval: 60 seconds (configurable)
10. Resilient error handling: continue on partial failures

## Non-Goals (Future Extensions)

- Account-level aggregated metrics (focus on zone-level initially)
- Workers metrics
- R2 storage metrics
- Logpush job metrics
- Colocation metrics

---

## Stage 1: Foundation & Configuration

**Goal**: Set up basic structure for metrics collection without breaking existing logs functionality
**Success Criteria**:

- Configuration validates correctly for metrics
- Factory registers metrics receiver
- All existing tests pass
- New metrics config documented

**Tasks**:

1. Update `config.go`:
   - Add `MetricsConfig` struct with fields:
     - `api_token` (configopaque.String, required): Cloudflare API token (supports env var: `${env:CF_API_TOKEN}`)
     - `account_id` (string, required): Cloudflare account ID (for zone discovery)
     - `zones` ([]string, optional): List of zone IDs to monitor (empty = all zones in account)
     - `exclude_zones` ([]string, optional): List of zone IDs to exclude from monitoring
     - `collection_interval` (time.Duration, default: 60s): How often to scrape metrics
     - `enable_zone_metrics` (bool, default: true): Enable per-zone HTTP analytics
     - `enable_firewall_metrics` (bool, default: true): Enable firewall/WAF metrics per zone
     - `include_country_dimension` (bool, default: false): Include country in firewall metrics (200x cardinality increase)
   - Add `Metrics` field to main `Config` struct (sibling to `Logs`)
   - Add validation for metrics config:
     - Require `api_token` if metrics configured
     - Require `account_id` if metrics configured
     - Validate that `zones` and `exclude_zones` are not both set
     - Validate that at least one of `Logs` or `Metrics` is configured
     - **Decision**: Continue on partial zone failures, log errors
   - Ensure backward compatibility with existing `LogsConfig`

2. Update `factory.go`:
   - Register metrics capability: `receiver.WithMetrics(createMetricsReceiver, metadata.MetricsStability)`
   - Implement `createMetricsReceiver` function
   - Update `createDefaultConfig` to include metrics defaults

3. Update `metadata.yaml`:
   - Add metrics stability: `development: [metrics]` (under status.stability)
   - Ensure logs remain at alpha

4. Update `doc.go`:
   - Keep `//go:generate mdatagen metadata.yaml`
   - Update package comment to mention metrics capability

5. Add initial documentation:
   - Update README.md with metrics configuration section
   - Document authentication (API token only)
   - Add example configuration showing:
     - Metrics only (all zones auto-discovered)
     - Metrics with explicit zone list
     - Metrics with zone exclusions
     - Logs and metrics together

**Tests**:

- Config validation for metrics config
- Factory creates metrics receiver
- Backward compatibility test for logs-only config

**Status**: Not Started

---

## Stage 2: Cloudflare SDK Integration & Client

**Goal**: Implement Cloudflare API client wrapper using cloudflare-go/v6 SDK
**Success Criteria**:

- Client successfully authenticates with Cloudflare API
- Client can list zones for an account
- Client handles API errors gracefully
- Unit tests with mocked API responses

**Tasks**:

1. Add dependency:
   - Run `go get github.com/cloudflare/cloudflare-go/v6`
   - Update `go.mod` and `go.sum`

2. Create `internal/client/cloudflare_client.go`:
   - Interface definition:

     ```go
     type CloudflareClient interface {
         // REST API methods (using cloudflare-go SDK)
         ListZones(ctx context.Context, accountID string) ([]Zone, error)
         GetAccountAnalytics(ctx context.Context, accountID string, params AnalyticsParams) (AccountAnalytics, error)
         GetZoneAnalytics(ctx context.Context, zoneID string, params AnalyticsParams) (ZoneAnalytics, error)

         // GraphQL API methods (using direct HTTP client)
         GetFirewallEventsByAction(ctx context.Context, zoneID string, params AnalyticsParams) (FirewallAnalytics, error)
         GetFirewallEventsBySource(ctx context.Context, zoneID string, params AnalyticsParams) (FirewallAnalytics, error)
     }
     ```

   - Implement REST client using cloudflare-go SDK
   - Implement GraphQL client using net/http (see GRAPHQL_RESEARCH.md)
   - Handle authentication with API token for both clients
   - Implement rate limiting and retries
   - Add logging for API calls
   - Implement zone filtering logic (zones/exclude_zones)

3. Create data models in `internal/client/models.go`:
   - `Zone` struct (ID, Name, Status)
   - `AccountAnalytics` struct (account-level bandwidth, requests, threats)
   - `ZoneAnalytics` struct (per-zone bandwidth, requests, threats, pageviews)
   - `FirewallAnalytics` struct:
     - Always includes: action, source, count
     - Optionally includes: country (if configured)
     - **NO** rule_id (extremely high cardinality)
     - Fields: action, source, country (nullable), count
   - `AnalyticsParams` struct (time ranges, filters)

4. Create `internal/client/graphql_client.go` (separate file for GraphQL):
   - Simple GraphQL client using net/http
   - Methods: `Query()`, `GetFirewallEvents()`
   - Type-safe response parsing
   - Error handling for GraphQL errors
   - See GRAPHQL_RESEARCH.md for implementation example

5. Create `internal/client/cloudflare_client_test.go`:
   - Test authentication (REST and GraphQL)
   - Test zone listing and auto-discovery
   - Test zone filtering (zones parameter)
   - Test zone exclusion (exclude_zones parameter)
   - Test account analytics fetching
   - Test zone analytics fetching
   - Test firewall analytics fetching (GraphQL)
   - Test error handling for both REST and GraphQL
   - Mock Cloudflare API responses (REST and GraphQL)

**Tests**:

- Unit tests for client methods
- Error handling tests
- Mock-based tests (no real API calls)

**Status**: Not Started

---

## Stage 3: Metrics Scraper Implementation

**Goal**: Implement metrics scraper that polls Cloudflare API and generates OTEL metrics
**Success Criteria**:

- Scraper successfully fetches analytics from Cloudflare
- Scraper converts Cloudflare data to OTEL metrics format
- Scraper handles errors and retries
- Integration test with mocked client

**Tasks**:

1. Create `metrics.go`:
   - Implement `metricsReceiver` struct with:
     - scraperhelper.ScraperControllerSettings
     - CloudflareClient
     - MetricsConfig
     - logger
   - Implement `newMetricsReceiver` function
   - Implement lifecycle methods: `Start`, `Scrape`, `Shutdown`

2. Create `scraper.go`:
   - Implement `scrape(ctx context.Context) (pmetric.Metrics, error)`:
     - Fetch account-level analytics if `enable_account_metrics` is true
     - Auto-discover zones from account (filter by `zones` or `exclude_zones`)
     - For each zone:
       - Fetch zone analytics if `enable_zone_metrics` is true (REST API)
       - Fetch firewall analytics if `enable_firewall_metrics` is true (GraphQL API)
     - Convert to OTEL metrics
   - Handle pagination if needed
   - Implement error handling and logging
   - Log zone discovery results (how many zones found/filtered)
   - Handle mixed success (some zones succeed, others fail)

3. Create metric conversion functions:
   - `accountAnalyticsToMetrics(accountID string, analytics AccountAnalytics) pmetric.Metrics`
     - Account metrics (with account_id as attribute):
       - `cloudflare.account.bandwidth.total` (Sum, bytes)
       - `cloudflare.account.requests.total` (Sum, count)
       - `cloudflare.account.requests.cached` (Sum, count)
       - `cloudflare.account.requests.uncached` (Sum, count)
       - `cloudflare.account.threats.total` (Sum, count)
   - `zoneAnalyticsToMetrics(zone Zone, analytics ZoneAnalytics) pmetric.Metrics`
     - Zone metrics (with zone_id, zone_name as attributes):
       - `cloudflare.zone.bandwidth.total` (Sum, bytes)
       - `cloudflare.zone.bandwidth.cached` (Sum, bytes)
       - `cloudflare.zone.bandwidth.uncached` (Sum, bytes)
       - `cloudflare.zone.requests.total` (Sum, count)
       - `cloudflare.zone.requests.cached` (Sum, count)
       - `cloudflare.zone.requests.uncached` (Sum, count)
       - `cloudflare.zone.threats.total` (Sum, count)
       - `cloudflare.zone.pageviews.total` (Sum, count)
   - `firewallAnalyticsToMetrics(zone Zone, analytics FirewallAnalytics, includeCountry bool) pmetric.Metrics`
     - Firewall/WAF metrics (with zone_id, zone_name as attributes):
       - `cloudflare.zone.firewall.requests` (Sum, count)
         - **Default dimensions**: action, source (~100 time series per zone)
           - Actions: allow, block, challenge, jschallenge, log, connectionClose (~10 values)
           - Sources: waf, firewallManaged, firewallRules, rateLimit, securityLevel (~10 values)
         - **Optional dimension** (if `include_country_dimension: true`): country (~20,000 time series per zone)
           - Countries: ISO country codes (~200 values)
     - **Note**: Country dimension opt-in to balance observability with cardinality management.

4. Handle time ranges:
   - Use `collection_interval` to determine time range for analytics
   - Store last scrape timestamp to avoid gaps/overlaps
   - Handle clock drift and API delays

**Tests**:

- Unit tests for metric conversion (account, zone, firewall)
- Test zone auto-discovery and filtering
- Test zone exclusion logic
- Integration test with mocked client (REST and GraphQL)
- Test error scenarios (API failures, partial data, GraphQL errors)
- Test metric attributes and values
- Test with account metrics disabled
- Test with zone/firewall metrics disabled
- Test low cardinality of firewall metrics (no rule_id dimension)

**Status**: Not Started

---

## Stage 4: Metrics Metadata Definition

**Goal**: Define metrics in metadata.yaml for code generation
**Success Criteria**:

- All metrics defined in metadata.yaml
- mdatagen generates correct code
- Metrics builder works correctly

**Tasks**:

1. Update `metadata.yaml`:
   - Add `resource_attributes` section:

     ```yaml
     resource_attributes:
       cloudflare.account.id:
         description: Cloudflare account ID
         type: string
       cloudflare.zone.id:
         description: Cloudflare zone ID
         type: string
       cloudflare.zone.name:
         description: Cloudflare zone name
         type: string
       cloudflare.firewall.action:
         description: Firewall action taken (allow, block, challenge, jschallenge, log, connectionClose)
         type: string
         enabled: true
       cloudflare.firewall.source:
         description: Source of firewall event (waf, firewallManaged, firewallRules, rateLimit, securityLevel)
         type: string
         enabled: true
       cloudflare.firewall.country:
         description: Client country (ISO code)
         type: string
         enabled: false  # Opt-in via config
     ```

   - Add `metrics` section with all account, zone, and firewall metrics
   - Define metric types (sum/gauge), units (bytes/count), descriptions
   - Firewall metric attributes:
     - **Always**: action (~10), source (~10)
     - **Optional**: country (~200) - enabled via `include_country_dimension: true`
   - **Cardinality**:
     - Default: ~100 time series per zone (action × source)
     - With country: ~20,000 time series per zone (action × source × country)
   - **Excluded**: rule_id (thousands of values), clientIP, userAgent (log analysis domain)

2. Run code generation:
   - Execute `go generate ./...` to generate metadata code
   - Verify generated files in `internal/metadata/`

3. Update scraper to use generated metric builders:
   - Replace manual metric construction with generated builders
   - Use `metadata.Metrics` builder pattern

**Tests**:

- Verify generated code compiles
- Test metric builders
- Validate metadata structure

**Status**: Not Started

---

## Stage 5: Integration & Testing

**Goal**: Complete integration testing and documentation
**Success Criteria**:

- Full integration test with collector
- Example configuration works end-to-end
- README.md complete and accurate
- All tests pass

**Tasks**:

1. Create `metrics_test.go`:
   - Test full receiver lifecycle
   - Test with sample configuration
   - Test metrics output format
   - Test concurrent scraping (multiple zones)

2. Create `metrics_integration_test.go`:
   - Mark with build tag `// go:build integration`
   - Test with real Cloudflare API (using test account)
   - Verify actual metrics collection

3. Add testdata:
   - Create `testdata/metrics_config.yaml` with example configuration
   - Create `testdata/cloudflare_api_responses.json` with sample responses

4. Update documentation:
   - Complete README.md with:
     - Metrics configuration section
     - List of available metrics (account, zone, WAF)
     - Authentication setup guide (API token)
     - Zone filtering examples (auto-discovery, explicit zones, exclude zones)
     - Example configurations (metrics only, logs+metrics, different metric combinations)
     - Troubleshooting section
   - Add architecture diagram (optional)

5. Add example collector config:
   - Create `testdata/collector_config.yaml` showing:
     - Cloudflare receiver with metrics
     - Prometheus exporter
     - Debug exporter
     - Full pipeline example

**Tests**:

- Integration tests pass
- Load test with multiple zones
- Error recovery test
- Configuration validation test

**Status**: Not Started

---

## Stage 6: Code Quality & Submission

**Goal**: Ensure code quality and prepare for PR submission
**Success Criteria**:

- All linters pass
- Code coverage >80%
- CHANGELOG updated
- PR ready for review

**Tasks**:

1. Code quality:
   - Run `make lint` and fix all issues
   - Run `make test` and ensure all tests pass
   - Run `make gofmt` and `make goimports`
   - Check code coverage: `make cover`

2. Documentation:
   - Update CHANGELOG.md with new features
   - Ensure all exported functions have godoc comments
   - Update README.md with metrics stability status

3. Verify contribution requirements (CONTRIBUTING.md):
   - [ ] Component code owner identified (current: @dehaansa)
   - [ ] Sponsor (approver/maintainer) needed - to be identified
   - [ ] All required tests present and passing
   - [ ] Documentation complete
   - [ ] No breaking changes to existing logs functionality

4. Prepare PR:
   - Create feature branch: `feature/cloudflare-receiver-metrics`
   - Commit with clear messages following conventional commits
   - Write comprehensive PR description with:
     - Summary of changes
     - Link to issue (if exists)
     - Testing methodology
     - Example configuration
     - Screenshots/output examples

5. Post-submission:
   - Address review comments
   - Update based on maintainer feedback
   - Ensure CI passes

**Tests**:

- All existing tests still pass
- New tests have good coverage
- CI pipeline succeeds

**Status**: Not Started

---

## Design Decisions

### 1. Configuration Structure

**Decision**: Add `MetricsConfig` as a sibling to `LogsConfig` in main `Config` struct
**Rationale**:

- Maintains clear separation of concerns
- Allows users to enable logs, metrics, or both independently
- Follows pattern seen in other multi-signal receivers
- Easy to extend with more config options later

### 2. Cloudflare SDK vs Direct API

**Decision**: Use `github.com/cloudflare/cloudflare-go/v6` SDK
**Rationale**:

- Official SDK from Cloudflare
- Handles authentication, rate limiting, retries
- Type-safe API interactions
- Actively maintained (v6 is latest)
- Reduces custom HTTP client code

### 3. Metric Naming Convention

**Decision**: Use `cloudflare.{category}.{metric}.{aggregation}` pattern
**Rationale**:

- Clear hierarchy and grouping
- Follows OTEL semantic conventions
- Examples: `cloudflare.zone.bandwidth.total`, `cloudflare.waf.requests.total`
- Easy to query and filter in backends

### 4. Scraper Pattern

**Decision**: Use `scraperhelper.Scraper` from collector framework
**Rationale**:

- Standard pattern for polling-based receivers
- Built-in collection interval handling
- Lifecycle management (start/stop)
- Error handling and metrics
- See azuremonitor receiver for reference

### 5. Time Range Strategy

**Decision**: Use collection_interval as time window for analytics queries
**Rationale**:

- Aligns with scraper cadence
- Avoids gaps in data collection
- Simple to understand and configure
- Cloudflare Analytics API supports arbitrary time ranges

### 6. Extensibility Design

**Decision**: Separate client interface and metric converters by category
**Rationale**:

- Easy to add new metric categories (Workers, R2, etc.) later
- Each category gets its own converter function
- Client interface can be extended without breaking existing code
- Follows Open/Closed Principle

### 7. Firewall Metrics Cardinality

**Decision**: Include `action` and `source` dimensions by default; make `country` opt-in via configuration
**Rationale**:

**Always Included**:

- **Action** (~10 values): block, allow, challenge, jschallenge, log, connectionClose - essential for alerting
- **Source** (~10 values): waf, firewallManaged, firewallRules, rateLimit - identifies attack vector

**Opt-In via `include_country_dimension: true`**:

- **Country** (~200 values): Enables geographic threat analysis, geofencing, compliance monitoring
- **Trade-off**: 200x cardinality increase (100 → 20,000 time series per zone)
- **Use case**: Users with geographic security requirements or who need compatibility with lablabs/cloudflare-exporter

**Always Excluded**:

- **rule_id** (thousands): Extremely high cardinality, forensics not operational monitoring
- **clientIP/userAgent**: Log analysis domain, not metrics

**Cardinality Summary**:

| Configuration | Per Zone | 100 Zones | Backend Requirements |
|--------------|----------|-----------|---------------------|
| Default (action + source) | ~100 | ~10,000 | Standard Prometheus |
| With country enabled | ~20,000 | ~2,000,000 | Prometheus + Thanos/Mimir |

**What to alert on (default)**:

- Spike in blocks overall → Attack detected
- Spike in challenges → Bot activity
- Rate changes → DDoS detection
- Source pattern changes → New attack vectors

**Additional alerts (if country enabled)**:

- Spike in blocks from specific country → Targeted geographic attack

---

## Open Questions

1. **Sponsor**: In progress - discussing with current code owner

## Resolved Questions

1. **Configuration Structure** ✓: `MetricsConfig` as sibling to `LogsConfig` in main `Config` struct (Option A)
2. **Collection Interval** ✓: Default 60 seconds (configurable via `collection_interval`)
3. **Error Handling** ✓: Continue on partial failures, emit metrics for successful zones, log errors (Option B)
4. **Config Validation** ✓: Require at least one of `logs` or `metrics` to be configured
5. **API Token Security** ✓: Support environment variables via `configopaque.String` (OTEL collector built-in support: `${env:CF_API_TOKEN}`)
6. **Account vs Zone Metrics** ✓: Focus on zone-level metrics initially. Account-level metrics moved to future extensions.
7. **Zone Discovery** ✓: Auto-discover all zones by default, support explicit `zones` filter and `exclude_zones` exclusion (matches lablabs/cloudflare-exporter)
8. **GraphQL Support** ✓: Implement direct HTTP client (no third-party library needed). See GRAPHQL_RESEARCH.md for details.
9. **Metric Cardinality** ✓: Default to action/source dimensions (~100 time series per zone). Country opt-in via config.
10. **WAF/Firewall Metrics** ✓: Include in initial implementation (~8-11 hours additional effort)
11. **Country Dimension** ✓: **Opt-in configuration** (`include_country_dimension: false` by default). Reasoning:
    - **Default (without country)**: Low cardinality (~100 per zone), works with standard Prometheus
    - **Opt-in (with country)**: Enables geographic threat analysis, geofencing, compliance
    - **User choice**: Balances observability needs with infrastructure capabilities
    - **Flexibility**: Users with proper backends (Thanos/Mimir) can enable if needed
    - Provides path for lablabs/cloudflare-exporter compatibility when required

---

## Dependencies

- `github.com/cloudflare/cloudflare-go/v6`: Cloudflare Go SDK
- `go.opentelemetry.io/collector/receiver/scraperhelper`: Scraper helper framework
- `go.opentelemetry.io/collector/pdata/pmetric`: OTEL metrics data structures

---

## Timeline Estimate

- Stage 1: 4-6 hours
- Stage 2: 14-19 hours (includes GraphQL client implementation)
  - REST client: 6-8 hours
  - GraphQL client: 8-11 hours
- Stage 3: 10-13 hours (includes firewall metrics conversion)
- Stage 4: 4-5 hours (includes firewall metrics metadata)
- Stage 5: 6-8 hours
- Stage 6: 4-6 hours

**Total**: ~42-57 hours (increased from 30-40 due to GraphQL/firewall metrics)

---

## References

- [CONTRIBUTING.md - Adding New Components](../../CONTRIBUTING.md#adding-new-components)
- [Cloudflare Analytics API](https://developers.cloudflare.com/analytics/)
- [Cloudflare GraphQL Analytics API](https://developers.cloudflare.com/analytics/graphql-api/)
- [Querying Firewall Events with GraphQL](https://developers.cloudflare.com/analytics/graphql-api/tutorials/querying-firewall-events/)
- [Cloudflare Go SDK v6](https://github.com/cloudflare/cloudflare-go)
- [Azure Monitor Receiver](../azuremonitorreceiver) - Reference implementation
- [lablabs/cloudflare-exporter](https://github.com/lablabs/cloudflare-exporter) - Inspiration
- [GRAPHQL_RESEARCH.md](./GRAPHQL_RESEARCH.md) - GraphQL implementation research and examples
