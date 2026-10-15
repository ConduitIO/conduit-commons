// Copyright © 2026 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package hostpolicy carries the Conduit operator's decision about ambient
// credentials from the engine to built-in connectors.
//
// "Ambient credentials" are the cloud identity of the process running Conduit:
// for AWS, the default credential chain (environment, shared config and SSO,
// web identity/IRSA, ECS and EC2 roles). A connector that falls back to them
// lets whoever writes a pipeline act as the Conduit host. The engine's API is
// unauthenticated by default, so that can be anyone who reaches it. This
// package is how the operator, not the pipeline author, decides whether that is
// allowed and within which limits.
//
// # Core types
//
//   - [Policy] is the operator's decision. The engine builds it from
//     conduit.Config at startup and puts it on the context of every call into a
//     built-in connector with [WithPolicy]. A connector reads it with
//     [FromContext].
//   - [Error] is a refusal with a stable [Code], the failing config path and a
//     suggested fix. The engine converts it into its own coded error at the
//     plugin boundary.
//   - [RedirectGuard] and [DialControl] are the transport-level checks a
//     connector installs on the HTTP client it signs requests with.
//
// # Invariants
//
//   - Absent means deny. [FromContext] returns the zero [Policy] when the
//     engine supplied none (a standalone plugin, a unit test, an older engine),
//     and every check on the zero Policy refuses. There is no "unset means
//     allow" state.
//   - A pipeline config cannot set the policy. Only code holding the context
//     (the engine) can call [WithPolicy]; connector parameters never reach it.
//   - Endpoints match exactly. There is no suffix or wildcard matching: hosts
//     under shared cloud suffixes (for example *.execute-api.<region>.amazonaws.com)
//     can be created by anyone, so a suffix rule would hand signed requests to
//     them.
//   - Under ambient credentials no connection is made to a link-local or cloud
//     metadata address, whatever the allowlist says (see [DialControl]).
//
// Enforcement is cooperative: the connector applies these checks to the client
// it builds. The package protects against a hostile pipeline author, not a
// hostile connector binary, which already runs with the host's privileges.
//
// The package has no cloud-provider dependency. Provider-specific work (loading
// the AWS default chain, checking the endpoint the AWS SDK resolved) stays in
// the connector.
//
// Design: ConduitIO/conduit docs/design-documents/20261012-builtin-connector-ambient-credentials.md.
package hostpolicy
