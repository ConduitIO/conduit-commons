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

package protobuf

import (
	"fmt"
	"sync/atomic"
	"time"
)

// DefaultCompileTimeout is the compile timeout used when neither
// WithCompileTimeout nor SetDefaultCompileTimeout set one. It is a
// conservative bound, not a measured one: large real-world schemas compile
// far faster, and an operator with a schema that needs longer can raise it.
const DefaultCompileTimeout = 5 * time.Second

// defaultCompileTimeout holds the process-wide timeout set by
// SetDefaultCompileTimeout, or 0 for DefaultCompileTimeout. It is atomic
// because Parse reads it from any goroutine that misses the schema.Serde
// cache.
var defaultCompileTimeout atomic.Int64

func currentDefaultCompileTimeout() time.Duration {
	if d := time.Duration(defaultCompileTimeout.Load()); d > 0 {
		return d
	}
	return DefaultCompileTimeout
}

// SetDefaultCompileTimeout changes the compile timeout Parse uses when it is
// not given WithCompileTimeout. That includes every compile reached through
// schema.Schema.Serde, which has no way to pass options. d must be > 0; the
// timeout can't be disabled.
//
// The change applies to compiles that start after the call. It does not
// affect Serdes already compiled and cached by schema.Schema.Serde, so call it
// early in the process, before any schema is parsed.
func SetDefaultCompileTimeout(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("%w: SetDefaultCompileTimeout requires d > 0, got %s", ErrInvalidOption, d)
	}
	defaultCompileTimeout.Store(int64(d))
	return nil
}

// DefaultMaxSchemaSize is the schema size cap used when neither
// WithMaxSchemaSize nor SetDefaultMaxSchemaSize set one: 1 MiB of .proto
// source, counting the schema and every schema it references. Real schemas
// are far smaller; the cap bounds the compile work a single schema can cause,
// including a compile that outlives its timeout (see the package doc).
const DefaultMaxSchemaSize = 1 << 20

// defaultMaxSchemaSize holds the process-wide cap set by
// SetDefaultMaxSchemaSize, or 0 for DefaultMaxSchemaSize.
var defaultMaxSchemaSize atomic.Int64

func currentDefaultMaxSchemaSize() int {
	if n := defaultMaxSchemaSize.Load(); n > 0 {
		return int(n)
	}
	return DefaultMaxSchemaSize
}

// SetDefaultMaxSchemaSize changes the schema size cap Parse uses when it is
// not given WithMaxSchemaSize, in bytes of .proto source including every
// referenced schema. n must be > 0; the cap can't be disabled. Like
// SetDefaultCompileTimeout, it applies to compiles that start after the call.
func SetDefaultMaxSchemaSize(n int) error {
	if n <= 0 {
		return fmt.Errorf("%w: SetDefaultMaxSchemaSize requires n > 0, got %d", ErrInvalidOption, n)
	}
	defaultMaxSchemaSize.Store(int64(n))
	return nil
}

// Option configures Parse.
type Option func(*options) error

type options struct {
	compileTimeout time.Duration // 0 = use defaultCompileTimeout
	maxSchemaSize  int           // 0 = use defaultMaxSchemaSize

	references []Reference
	resolve    ResolveFunc

	// sourceHook, if set, runs each time the compiler reads the schema
	// source. Tests use it to hold a compile open past its deadline; it is
	// unexported because it is not a supported extension point.
	sourceHook func()
}

func resolveOptions(opts []Option) (options, error) {
	var o options
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return options{}, err
		}
	}
	if o.compileTimeout == 0 {
		o.compileTimeout = currentDefaultCompileTimeout()
	}
	if o.maxSchemaSize == 0 {
		o.maxSchemaSize = currentDefaultMaxSchemaSize()
	}
	return o, nil
}

// WithCompileTimeout sets the compile timeout for one Parse call, overriding
// the process-wide default. d must be > 0; the timeout can't be disabled.
func WithCompileTimeout(d time.Duration) Option {
	return func(o *options) error {
		if d <= 0 {
			return fmt.Errorf("%w: WithCompileTimeout requires d > 0, got %s", ErrInvalidOption, d)
		}
		o.compileTimeout = d
		return nil
	}
}

// WithMaxSchemaSize sets the schema size cap for one Parse call, overriding
// the process-wide default. n must be > 0; the cap can't be disabled.
func WithMaxSchemaSize(n int) Option {
	return func(o *options) error {
		if n <= 0 {
			return fmt.Errorf("%w: WithMaxSchemaSize requires n > 0, got %d", ErrInvalidOption, n)
		}
		o.maxSchemaSize = n
		return nil
	}
}

// WithReferences declares the schema's references and the function that
// fetches them. Parse resolves them, and their own references, before
// compiling. A schema with references and a nil resolve fails with
// ErrReferenceResolve.
func WithReferences(refs []Reference, resolve ResolveFunc) Option {
	return func(o *options) error {
		o.references = append([]Reference(nil), refs...)
		o.resolve = resolve
		return nil
	}
}
