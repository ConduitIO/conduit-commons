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

// Option configures Parse.
type Option func(*options) error

type options struct {
	compileTimeout time.Duration // 0 = use defaultCompileTimeout

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
