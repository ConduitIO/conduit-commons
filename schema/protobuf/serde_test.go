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
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matryer/is"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// sentinels lists every error Parse may return; each error must match
// exactly one of them.
var sentinels = []error{ErrInvalidOption, ErrCompileTimeout, ErrReferencesNotSupported, ErrSchemaCompile}

func matchingSentinels(err error) []error {
	var out []error
	for _, s := range sentinels {
		if errors.Is(err, s) {
			out = append(out, s)
		}
	}
	return out
}

func messageNames(fd protoreflect.FileDescriptor) []string {
	var names []string
	var walk func(protoreflect.MessageDescriptors)
	walk = func(mds protoreflect.MessageDescriptors) {
		for i := 0; i < mds.Len(); i++ {
			md := mds.Get(i)
			names = append(names, string(md.FullName()))
			walk(md.Messages())
		}
	}
	walk(fd.Messages())
	return names
}

func TestParse_Valid(t *testing.T) {
	testCases := []struct {
		name         string
		text         string
		wantMessages []string
	}{{
		name:         "flat single message",
		text:         flatSchema,
		wantMessages: []string{"example.v1.Order"},
	}, {
		name: "several top-level and nested messages",
		text: multipleMessagesSchema,
		wantMessages: []string{
			"example.v1.Customer",
			"example.v1.Customer.Address",
			"example.v1.Invoice",
		},
	}, {
		name:         "standard import is not a reference",
		text:         wellKnownImportSchema,
		wantMessages: []string{"example.v1.Event"},
	}, {
		name:         "proto2",
		text:         proto2Schema,
		wantMessages: []string{"example.v1.Legacy"},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			srd, err := Parse([]byte(tc.text))
			is.NoErr(err)
			is.Equal(srd.String(), tc.text)
			is.Equal(messageNames(srd.file), tc.wantMessages)
		})
	}
}

func TestParse_WellKnownTypeIsLinked(t *testing.T) {
	is := is.New(t)

	srd, err := Parse([]byte(wellKnownImportSchema))
	is.NoErr(err)

	field := srd.file.Messages().ByName("Event").Fields().ByName("created_at")
	is.True(field != nil)
	is.Equal(field.Message().FullName(), protoreflect.FullName("google.protobuf.Timestamp"))
}

func TestParse_Errors(t *testing.T) {
	testCases := []struct {
		name        string
		text        string
		wantErr     error
		wantMessage string
	}{{
		name:        "syntax error carries its position",
		text:        malformedSchema,
		wantErr:     ErrSchemaCompile,
		wantMessage: "schema.proto:7:",
	}, {
		name:        "duplicate field number",
		text:        duplicateFieldNumberSchema,
		wantErr:     ErrSchemaCompile,
		wantMessage: "same tag 1",
	}, {
		name:        "unknown type",
		text:        unknownTypeSchema,
		wantErr:     ErrSchemaCompile,
		wantMessage: "DoesNotExist",
	}, {
		name:        "import of a non-standard file is a reference",
		text:        referenceSchema,
		wantErr:     ErrReferencesNotSupported,
		wantMessage: `"example/v1/customer.proto"`,
	}, {
		name:        "self import is a cycle",
		text:        selfImportSchema,
		wantErr:     ErrSchemaCompile,
		wantMessage: "cycle",
	}, {
		name:    "not proto at all",
		text:    `{"type": "record", "name": "avro"}`,
		wantErr: ErrSchemaCompile,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			srd, err := Parse([]byte(tc.text))
			is.True(srd == nil)
			is.True(err != nil)
			is.Equal(matchingSentinels(err), []error{tc.wantErr})
			if tc.wantMessage != "" && !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("error %q does not contain %q", err, tc.wantMessage)
			}
		})
	}
}

// TestParse_CompileTimeout holds the compile open (the compiler blocks while
// reading the schema source) until well after the deadline, then checks that
// Parse returned on time with an error schema.Schema.Serde won't cache.
func TestParse_CompileTimeout(t *testing.T) {
	is := is.New(t)

	release := make(chan struct{})
	hook := func(o *options) error {
		o.sourceHook = func() { <-release }
		return nil
	}

	start := time.Now()
	srd, err := Parse([]byte(flatSchema), WithCompileTimeout(20*time.Millisecond), hook)
	elapsed := time.Since(start)
	close(release) // let the orphaned compile goroutine finish

	is.True(srd == nil)
	is.Equal(matchingSentinels(err), []error{ErrCompileTimeout})
	is.True(errors.Is(err, context.DeadlineExceeded))
	is.True(strings.Contains(err.Error(), "20ms"))
	if elapsed > 2*time.Second {
		t.Fatalf("Parse returned after %s, want it bounded by the 20ms timeout", elapsed)
	}
}

func TestSetDefaultCompileTimeout(t *testing.T) {
	is := is.New(t)
	t.Cleanup(func() { defaultCompileTimeout.Store(0) })

	is.Equal(currentDefaultCompileTimeout(), DefaultCompileTimeout)
	is.NoErr(SetDefaultCompileTimeout(20 * time.Millisecond))

	release := make(chan struct{})
	defer close(release)
	hook := func(o *options) error {
		o.sourceHook = func() { <-release }
		return nil
	}

	_, err := Parse([]byte(flatSchema), hook)
	is.True(errors.Is(err, ErrCompileTimeout))
	is.True(strings.Contains(err.Error(), "20ms"))

	// a per-call option overrides the process-wide default
	o, err := resolveOptions([]Option{WithCompileTimeout(time.Minute)})
	is.NoErr(err)
	is.Equal(o.compileTimeout, time.Minute)
}

func TestCompileTimeout_CannotBeDisabled(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			is := is.New(t)
			t.Cleanup(func() { defaultCompileTimeout.Store(0) })

			_, err := Parse([]byte(flatSchema), WithCompileTimeout(d))
			is.Equal(matchingSentinels(err), []error{ErrInvalidOption})

			err = SetDefaultCompileTimeout(d)
			is.True(errors.Is(err, ErrInvalidOption))
			is.Equal(currentDefaultCompileTimeout(), DefaultCompileTimeout) // unchanged
		})
	}
}

func TestParse_DoesNotRetainInput(t *testing.T) {
	is := is.New(t)

	text := []byte(flatSchema)
	srd, err := Parse(text)
	is.NoErr(err)

	for i := range text {
		text[i] = 'x'
	}
	is.Equal(srd.String(), flatSchema)
}

func TestParse_Concurrent(t *testing.T) {
	is := is.New(t)

	var wg sync.WaitGroup
	errs := make([]error, 16)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = Parse([]byte(multipleMessagesSchema))
		}()
	}
	wg.Wait()
	for _, err := range errs {
		is.NoErr(err)
	}
}

func TestSerde_IsDecodeOnly(t *testing.T) {
	is := is.New(t)

	srd, err := Parse([]byte(flatSchema))
	is.NoErr(err)

	out, err := srd.Marshal(map[string]any{"id": "1"})
	is.True(out == nil)
	is.True(errors.Is(err, ErrEncodingNotSupported))

	// Message index [0] (shortcut), then the payload.
	var v any
	err = srd.Unmarshal([]byte{0x00, 0x0a, 0x01, 0x31}, &v)
	is.Equal(matchingUnmarshalSentinels(err), []error{ErrDecodeNotImplemented})
	is.Equal(v, nil)
}
