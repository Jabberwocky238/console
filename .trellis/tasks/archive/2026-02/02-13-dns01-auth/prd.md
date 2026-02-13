# Implement DNS-01 Authentication using jw238dns Submodule

## Overview

Integrate the jw238dns submodule to provide DNS-01 challenge support for custom domain verification. Replace the current `net.LookupTXT` verification with jw238dns HTTP API calls to manage TXT records dynamically.

## Current Architecture

**Existing Flow**:
1. User adds custom domain via Outer Gateway
2. System generates TXT record name and value (`_combinator-verify.example.com`)
3. User manually creates TXT record in their DNS provider
4. System polls DNS using `net.LookupTXT` to verify
5. After verification, creates IngressRoute and Certificate

**Problem**: Users must manually create TXT records in their DNS provider, which is cumbersome.

## New Architecture with jw238dns

**New Flow**:
1. User adds custom domain via Outer Gateway
2. System generates TXT record name and value
3. **Outer Gateway calls jw238dns HTTP API to create TXT record automatically**
4. System polls jw238dns DNS server to verify (or uses `net.LookupTXT` against jw238dns)
5. After verification, creates IngressRoute and Certificate
6. **Outer Gateway calls jw238dns HTTP API to delete TXT record after verification**

## Requirements

### 1. jw238dns Client Package

Create a new package `jw238dns/client` in the console project:

```go
package jw238dns

type Client struct {
    baseURL string
    httpClient *http.Client
}

func NewClient(baseURL string) *Client
func (c *Client) AddTXTRecord(domain, value string, ttl uint32) error
func (c *Client) DeleteTXTRecord(domain string) error
func (c *Client) GetTXTRecord(domain string) ([]string, error)
```

### 2. Update CustomDomain Verification

Modify `k8s/customdomain.go`:

- Add jw238dns client initialization
- Replace manual TXT verification with automated TXT record creation
- Call jw238dns API to create TXT record in `NewCustomDomain`
- Call jw238dns API to delete TXT record after verification succeeds
- Update `VerifyTXT` to query jw238dns DNS server

### 3. Configuration

Add environment variables:

- `JW238DNS_API_URL` - jw238dns HTTP API endpoint (e.g., `http://jw238dns.console.svc.cluster.local:8080`)
- `JW238DNS_DNS_SERVER` - jw238dns DNS server address (e.g., `jw238dns.console.svc.cluster.local:53`)

### 4. Error Handling

- Handle jw238dns API failures gracefully
- Fallback to manual verification if jw238dns is unavailable
- Log all jw238dns API calls for debugging

## Acceptance Criteria

- [ ] jw238dns client package created with HTTP API methods
- [ ] CustomDomain automatically creates TXT records via jw238dns API
- [ ] Verification queries jw238dns DNS server
- [ ] TXT records are cleaned up after successful verification
- [ ] Configuration via environment variables
- [ ] Error handling and logging implemented
- [ ] Manual verification fallback works if jw238dns unavailable

## Technical Notes

### jw238dns HTTP API Endpoints

Based on `jw238dns/http/handler_dns.go`:

- `POST /dns/add` - Add DNS record
  ```json
  {
    "domain": "_combinator-verify.example.com.",
    "type": "TXT",
    "value": ["combinator-verify=abc123"],
    "ttl": 300
  }
  ```

- `POST /dns/delete` - Delete DNS record
  ```json
  {
    "domain": "_combinator-verify.example.com.",
    "type": "TXT"
  }
  ```

- `GET /dns/get?domain=_combinator-verify.example.com.&type=TXT` - Get DNS record

### DNS Query

The verification can either:
1. Query jw238dns DNS server directly using custom DNS resolver
2. Continue using `net.LookupTXT` if jw238dns is configured as authoritative DNS

### Deployment Considerations

- jw238dns must be deployed in the same cluster
- Service name: `jw238dns.console.svc.cluster.local`
- HTTP API port: 8080
- DNS port: 53

## Out of Scope

- jw238dns deployment configuration (assumed to be already deployed)
- Web frontend changes (Terminal UI already supports custom domains)
- Database schema changes (existing schema is sufficient)
- cert-manager configuration changes

## Files to Modify

1. **New**: `jw238dns/client.go` - jw238dns HTTP client
2. **Modify**: `k8s/customdomain.go` - Integrate jw238dns client
3. **Modify**: `cmd/outer/main.go` - Add jw238dns configuration
4. **Modify**: `handlers/customdomain.handler.go` - Update error messages (optional)

## Testing Plan

1. Unit tests for jw238dns client
2. Integration test: Create custom domain → Verify TXT record created in jw238dns
3. Integration test: Successful verification → TXT record deleted
4. Integration test: Failed verification → TXT record remains for debugging
5. Manual test: Deploy to K8s and verify end-to-end flow
