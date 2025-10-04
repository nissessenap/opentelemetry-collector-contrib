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
2. Support Account-level analytics (requests, bandwidth, threats)
3. Support Zone analytics (bandwidth, requests, threats, pageviews)
4. Support WAF analytics (events, rules, actions)
5. Use Cloudflare Go SDK v6 for all API interactions
6. Auto-discover all zones in account by default with zone filtering support
7. Design for extensibility (easy to add Workers, R2, Logpush metrics later)
8. Maintain separation from existing logs functionality

## Non-Goals (Future Extensions)
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
     - `api_token` (string, required): Cloudflare API token
     - `account_id` (string, required): Cloudflare account ID
     - `zones` ([]string, optional): List of zone IDs to monitor (empty = all zones in account)
     - `exclude_zones` ([]string, optional): List of zone IDs to exclude from monitoring
     - `enable_account_metrics` (bool, default: true): Enable account-level metrics
     - `enable_zone_metrics` (bool, default: true): Enable per-zone metrics
     - `enable_waf_metrics` (bool, default: true): Enable WAF metrics per zone
   - Add validation for metrics config:
     - Require `api_token`
     - Require `account_id`
     - Validate that `zones` and `exclude_zones` are not both set
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
         ListZones(ctx context.Context, accountID string) ([]Zone, error)
         GetAccountAnalytics(ctx context.Context, accountID string, params AnalyticsParams) (AccountAnalytics, error)
         GetZoneAnalytics(ctx context.Context, zoneID string, params AnalyticsParams) (ZoneAnalytics, error)
         GetWAFAnalytics(ctx context.Context, zoneID string, params AnalyticsParams) (WAFAnalytics, error)
     }
     ```
   - Implement client using cloudflare-go SDK
   - Handle authentication with API token
   - Implement rate limiting and retries
   - Add logging for API calls
   - Implement zone filtering logic (zones/exclude_zones)

3. Create data models in `internal/client/models.go`:
   - `Zone` struct (ID, Name, Status)
   - `AccountAnalytics` struct (account-level bandwidth, requests, threats)
   - `ZoneAnalytics` struct (per-zone bandwidth, requests, threats, pageviews)
   - `WAFAnalytics` struct (events, rules, actions)
   - `AnalyticsParams` struct (time ranges, filters)

4. Create `internal/client/cloudflare_client_test.go`:
   - Test authentication
   - Test zone listing and auto-discovery
   - Test zone filtering (zones parameter)
   - Test zone exclusion (exclude_zones parameter)
   - Test account analytics fetching
   - Test zone analytics fetching
   - Test WAF analytics fetching
   - Test error handling
   - Mock Cloudflare API responses

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
       - Fetch zone analytics if `enable_zone_metrics` is true
       - Fetch WAF analytics if `enable_waf_metrics` is true
     - Convert to OTEL metrics
   - Handle pagination if needed
   - Implement error handling and logging
   - Log zone discovery results (how many zones found/filtered)

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
   - `wafAnalyticsToMetrics(zone Zone, analytics WAFAnalytics) pmetric.Metrics`
     - WAF metrics (with zone_id, zone_name, action, rule attributes):
       - `cloudflare.waf.requests.total` (Sum, count)
       - `cloudflare.waf.requests.by_action` (Sum, count, dimension: action)
       - `cloudflare.waf.requests.by_rule` (Sum, count, dimension: rule_id)

4. Handle time ranges:
   - Use `collection_interval` to determine time range for analytics
   - Store last scrape timestamp to avoid gaps/overlaps
   - Handle clock drift and API delays

**Tests**:
- Unit tests for metric conversion (account, zone, WAF)
- Test zone auto-discovery and filtering
- Test zone exclusion logic
- Integration test with mocked client
- Test error scenarios (API failures, partial data)
- Test metric attributes and values
- Test with account metrics disabled
- Test with zone/WAF metrics disabled

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
     ```
   - Add `metrics` section with all account, zone, and WAF metrics
   - Define metric types (sum/gauge), units (bytes/count), descriptions
   - Add optional attributes (action, rule_id, etc.)

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

---

## Open Questions
1. **Sponsor**: Need to identify a maintainer/approver to sponsor this contribution
2. **Metric Cardinality**: Should WAF metrics include all rule IDs as dimensions? (May create high cardinality)
3. **Rate Limiting**: What's the appropriate default collection_interval to avoid Cloudflare rate limits? (Recommendation: 60s)
4. **Error Handling**: Should partial failures (one zone fails) fail entire scrape or just log error? (Recommendation: Log and continue)

## Resolved Questions
1. **Zone Discovery** ✓: Auto-discover all zones by default, support explicit `zones` filter and `exclude_zones` exclusion (matches lablabs/cloudflare-exporter)
2. **Account Metrics** ✓: Support account-level metrics out of the box (matches lablabs/cloudflare-exporter)

---

## Dependencies
- `github.com/cloudflare/cloudflare-go/v6`: Cloudflare Go SDK
- `go.opentelemetry.io/collector/receiver/scraperhelper`: Scraper helper framework
- `go.opentelemetry.io/collector/pdata/pmetric`: OTEL metrics data structures

---

## Timeline Estimate
- Stage 1: 4-6 hours
- Stage 2: 6-8 hours
- Stage 3: 8-10 hours
- Stage 4: 3-4 hours
- Stage 5: 6-8 hours
- Stage 6: 4-6 hours

**Total**: ~30-40 hours

---

## References
- [CONTRIBUTING.md - Adding New Components](../../CONTRIBUTING.md#adding-new-components)
- [Cloudflare Analytics API](https://developers.cloudflare.com/analytics/)
- [Cloudflare Go SDK v6](https://github.com/cloudflare/cloudflare-go)
- [Azure Monitor Receiver](../azuremonitorreceiver) - Reference implementation
- [lablabs/cloudflare-exporter](https://github.com/lablabs/cloudflare-exporter) - Inspiration