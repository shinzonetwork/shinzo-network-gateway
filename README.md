# Shinzo Network Gateway

[![Go](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-test.yml/badge.svg)](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-test.yml)
[![golangci-lint](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-lint.yml/badge.svg)](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/shinzonetwork/shinzo-network-gateway)](https://goreportcard.com/report/github.com/shinzonetwork/shinzo-network-gateway)

The Shinzo Network Gateway is the primary entry point through which users interact with the Shinzo network.
It serves as a trustless routing and coordination layer - responsible for resolving which hosts serve a given piece of data, routing queries to those hosts, validating responses, and maintaining network integrity through cryptographic and economic mechanisms.

## GraphQL extensions

The gateway is GraphQL-over-HTTP compliant and accepts the standard request body fields: `query`, `operationName`, `variables`, and `extensions`. Any `extensions` you send on a request are forwarded verbatim to upstream hosts; the gateway itself does not interpret them.

On the response side, the gateway routes each query to multiple upstream hosts and compares their answers. Every successful response carries an `extensions.consensus` object that tells you how much the hosts agreed.

### `extensions.consensus`

`consensus` is a string with one of three values:

- **`full`** — every host that responded returned the same answer. The result is fully agreed upon.
- **`partial`** — hosts disagreed, but one response was strictly more popular than the others. The gateway returns the majority answer in `data`/`errors`.
- **`none`** — the top two response groups were tied. The gateway returns the first-seen response in `data`/`errors`, but no group has a majority and the result should be treated with caution.

Hosts that fail to respond (network errors, non-2xx status, oversized bodies) are excluded from the comparison. If every host fails, the gateway returns an error instead of a GraphQL response.

### `extensions.responses`

When `consensus` is `partial` or `none`, the gateway also includes a `responses` array so clients can see exactly what each host returned and decide for themselves whether to trust the chosen answer. Each entry contains:

- `response` — the raw response body returned by a group of hosts.
- `hosts` — the URLs of the hosts that returned that exact response.

Groups are ordered by how many hosts returned each response, most popular first. The first entry is always the response that was promoted to the top level of the gateway reply.

When `consensus` is `full`, the `responses` field is omitted, since every host returned the same answer.

### Examples

Full consensus:

```json
{
  "data": { "hero": { "name": "Luke" } },
  "extensions": { "consensus": "full" }
}
```

Partial consensus (2 hosts vs 1):

```json
{
  "data": { "a": 1 },
  "extensions": {
    "consensus": "partial",
    "responses": [
      { "response": { "data": { "a": 1 } }, "hosts": ["https://host-a", "https://host-b"] },
      { "response": { "data": { "a": 2 } }, "hosts": ["https://host-c"] }
    ]
  }
}
```

No consensus (1 vs 1 tie):

```json
{
  "data": { "a": 1 },
  "extensions": {
    "consensus": "none",
    "responses": [
      { "response": { "data": { "a": 1 } }, "hosts": ["https://host-a"] },
      { "response": { "data": { "a": 2 } }, "hosts": ["https://host-b"] }
    ]
  }
}
```

### Recommended client handling

- `full` — trust the response.
- `partial` — trust the response by default, but inspect `responses` if disagreement among hosts is meaningful for your use case.
- `none` — the gateway has picked one answer arbitrarily from a tie. Inspect `responses` and decide which to accept, or retry.
