# Shinzo Network Gateway

[![Go](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-test.yml/badge.svg)](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-test.yml)
[![golangci-lint](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-lint.yml/badge.svg)](https://github.com/shinzonetwork/shinzo-network-gateway/actions/workflows/go-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/shinzonetwork/shinzo-network-gateway)](https://goreportcard.com/report/github.com/shinzonetwork/shinzo-network-gateway)

The Shinzo Network Gateway is the primary entry point through which users interact with the Shinzo network.
It serves as a trustless routing and coordination layer - responsible for resolving which hosts serve a given piece of data, routing queries to those hosts, validating responses, and maintaining network integrity through cryptographic and economic mechanisms.


> **Status: prototype / intensive development.** Everything — APIs, configuration, behavior, on-disk formats — can change at any time without notice. Not ready for production use.

## Build

```sh
go build ./...
go test ./...
```

## Run

```sh
go run ./cmd/gateway
```

The gateway accepts GraphQL-over-HTTP requests, parses the root collections, and forwards each request to an online host that serves them.
