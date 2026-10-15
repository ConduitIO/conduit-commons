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

package hostpolicy

import (
	"errors"
)

// Code is a stable, machine-readable reason for a refusal. The string values
// are a public contract: the engine registers each one as an error code, and
// agents and operators match on them. Never change a value; add a new one.
type Code string

const (
	// CodeAmbientCredentialsDisabled: no static credentials were configured and
	// the operator has not enabled ambient credentials (or the engine supplied
	// no policy at all).
	CodeAmbientCredentialsDisabled Code = "connector.ambient_credentials_disabled"
	// CodeCredentialsIncomplete: exactly one of a static credential pair is set.
	// This never falls through to ambient credentials, whatever the policy says.
	CodeCredentialsIncomplete Code = "connector.credentials_incomplete"
	// CodeEndpointNotAllowed: under ambient credentials, the endpoint is not
	// https or is not an exact entry of the operator's endpoint allowlist.
	CodeEndpointNotAllowed Code = "connector.endpoint_not_allowed"
	// CodeRedirectRefused: a server answered with a redirect to a different
	// host or to a non-https URL. The request was not followed.
	CodeRedirectRefused Code = "connector.redirect_refused"
	// CodeEndpointLinkLocal: the connection would go to a link-local or cloud
	// metadata address, which is never permitted under ambient credentials.
	CodeEndpointLinkLocal Code = "connector.endpoint_link_local"
	// CodeResourceNotAllowed: the resource (for S3, the bucket) is not an exact
	// entry of the operator's resource allowlist, or is a reserved form (an ARN
	// or an alias) that a name allowlist cannot reason about.
	CodeResourceNotAllowed Code = "connector.resource_not_allowed"
	// CodeProxyNotAllowed: an HTTP proxy is configured in the process
	// environment and the operator has not opted in with allow-proxy.
	CodeProxyNotAllowed Code = "connector.proxy_not_allowed"
)

// Codes returns every refusal code this package defines, so the engine can
// register them all and a test can assert none is missing.
func Codes() []Code {
	return []Code{
		CodeAmbientCredentialsDisabled,
		CodeCredentialsIncomplete,
		CodeEndpointNotAllowed,
		CodeRedirectRefused,
		CodeEndpointLinkLocal,
		CodeResourceNotAllowed,
		CodeProxyNotAllowed,
	}
}

// Outcome classifies a [Decision] for metrics. Values are metric label values
// and are a public contract.
type Outcome string

const (
	OutcomeUsed             Outcome = "used"
	OutcomeDeniedDisabled   Outcome = "denied_disabled"
	OutcomeDeniedEndpoint   Outcome = "denied_endpoint"
	OutcomeDeniedRedirect   Outcome = "denied_redirect"
	OutcomeDeniedLinkLocal  Outcome = "denied_link_local"
	OutcomeDeniedResource   Outcome = "denied_resource"
	OutcomeDeniedProxy      Outcome = "denied_proxy"
	outcomeNotAmbientDenial Outcome = ""
)

// Outcome returns the metric outcome for a refusal with this code. It returns
// "" for CodeCredentialsIncomplete, which is a static-credential config error
// rather than an ambient-credentials decision.
func (c Code) Outcome() Outcome {
	switch c {
	case CodeAmbientCredentialsDisabled:
		return OutcomeDeniedDisabled
	case CodeEndpointNotAllowed:
		return OutcomeDeniedEndpoint
	case CodeRedirectRefused:
		return OutcomeDeniedRedirect
	case CodeEndpointLinkLocal:
		return OutcomeDeniedLinkLocal
	case CodeResourceNotAllowed:
		return OutcomeDeniedResource
	case CodeProxyNotAllowed:
		return OutcomeDeniedProxy
	case CodeCredentialsIncomplete:
		return outcomeNotAmbientDenial
	}
	return outcomeNotAmbientDenial
}

// defaultSuggestions holds the suggested fix for each code. Connectors may
// replace it with a more specific one (for example naming their own keys).
var defaultSuggestions = map[Code]string{
	CodeAmbientCredentialsDisabled: "set static credentials in the connector config, or ask the operator to set " +
		"connectors.ambient-credentials.enabled=true (and use the builtin: plugin prefix, for example builtin:s3)",
	CodeCredentialsIncomplete: "set both keys for static credentials, or neither to use ambient credentials " +
		"(if the operator allows them)",
	CodeEndpointNotAllowed: "use static credentials with this endpoint, or ask the operator to add it to " +
		"connectors.ambient-credentials.allow-endpoints",
	CodeRedirectRefused: "none for the pipeline author; report the endpoint to the operator",
	CodeEndpointLinkLocal: "use a different endpoint; link-local and metadata addresses are never permitted " +
		"with ambient credentials",
	CodeResourceNotAllowed: "use an allowed plain resource name, or ask the operator to extend " +
		"connectors.ambient-credentials.allow-resources",
	CodeProxyNotAllowed: "ask the operator to unset the proxy variables or set " +
		"connectors.ambient-credentials.allow-proxy=true",
}

// Error is a refusal. It carries a stable [Code], the connector config path
// that caused it (set by the connector with [Error.At]) and a suggested fix.
// Messages never contain credential values.
//
// Error reports itself as not retryable through the RetryableError method, an
// optional interface the AWS SDK's retryer checks before its connection-error
// rules. A refused redirect or dial therefore ends the request instead of
// being retried against the same server.
type Error struct {
	Code       Code
	ConfigPath string
	Message    string
	Suggestion string
}

// NewError returns an Error with the default suggestion for code.
func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message, Suggestion: defaultSuggestions[code]}
}

// Error returns the message prefixed with the code, so the code survives when
// the error is flattened to a string (for example across a plugin boundary).
func (e *Error) Error() string {
	if e.ConfigPath != "" {
		return string(e.Code) + ": " + e.ConfigPath + ": " + e.Message
	}
	return string(e.Code) + ": " + e.Message
}

// RetryableError always reports false: a policy refusal does not change on
// retry. See the type comment.
func (e *Error) RetryableError() bool { return false }

// At returns a copy of e with ConfigPath set to path.
func (e *Error) At(path string) *Error {
	cp := *e
	cp.ConfigPath = path
	return &cp
}

// AsError returns the first *Error in err's chain.
func AsError(err error) (*Error, bool) {
	var he *Error
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}
