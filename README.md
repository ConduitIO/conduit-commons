# Conduit Commons

[![License](https://img.shields.io/badge/license-Apache%202-blue)](https://github.com/ConduitIO/conduit-commons/blob/main/LICENSE.md)
[![Test](https://github.com/ConduitIO/conduit-commons/actions/workflows/test.yml/badge.svg)](https://github.com/ConduitIO/conduit-commons/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/conduitio/conduit-commons)](https://goreportcard.com/report/github.com/conduitio/conduit-commons)
[![Go Reference](https://pkg.go.dev/badge/github.com/conduitio/conduit-commons.svg)](https://pkg.go.dev/github.com/conduitio/conduit-commons)

Conduit Commons is a module containing shared code between
[Conduit](https://github.com/conduitio/conduit) and other related Conduit
repositories like [Conduit Connector SDK](https://github.com/conduitio/conduit-connector-sdk)
and [Conduit Processor SDK](https://github.com/conduitio/conduit-processor-sdk).

Check the [Go Reference](https://pkg.go.dev/github.com/conduitio/conduit-commons)
documentation for more information on the included packages.
## Property tests

Property tests use [rapid](https://pkg.go.dev/pgregory.net/rapid). Shared
generators for Avro schemas and values live in
[`schema/avro/avrotest`](schema/avro/avrotest), which other modules can import
from their tests. `go test` runs each property 100 times; set `RAPID_CHECKS`
(or pass `-rapid.checks`) to run more. A failure prints the `-rapid.seed` that
replays it. The `property-nightly` workflow runs every test whose name
contains `Property` with `RAPID_CHECKS=20000`.
