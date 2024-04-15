// Copyright © 2024 Meroxa, Inc.
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

package config

import (
	"errors"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/matryer/is"
)

func TestConfig_Sanitize(t *testing.T) {
	is := is.New(t)
	have := Config{"   key   ": "   value   "}
	want := Config{"key": "value"}

	have.Sanitize()
	is.Equal(have, want)
}

func TestConfig_Validate_ParameterType(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		params  Parameters
		wantErr bool
	}{{
		name:    "valid type number",
		config:  Config{"param1": "3"},
		params:  Parameters{"param1": {Default: "3.3", Type: ParameterTypeFloat}},
		wantErr: false,
	}, {
		name:    "invalid type float",
		config:  Config{"param1": "not-a-number"},
		params:  Parameters{"param1": {Default: "3.3", Type: ParameterTypeFloat}},
		wantErr: true,
	}, {
		name:    "valid default type float",
		config:  Config{"param1": ""},
		params:  Parameters{"param1": {Default: "3", Type: ParameterTypeFloat}},
		wantErr: false,
	}, {
		name:    "valid type int",
		config:  Config{"param1": "3"},
		params:  Parameters{"param1": {Type: ParameterTypeInt}},
		wantErr: false,
	}, {
		name:    "invalid type int",
		config:  Config{"param1": "3.3"},
		params:  Parameters{"param1": {Type: ParameterTypeInt}},
		wantErr: true,
	}, {
		name:    "valid type bool",
		config:  Config{"param1": "1"},
		params:  Parameters{"param1": {Type: ParameterTypeBool}},
		wantErr: false,
	}, {
		name:    "valid type bool",
		config:  Config{"param1": "true"},
		params:  Parameters{"param1": {Type: ParameterTypeBool}},
		wantErr: false,
	}, {
		name:    "invalid type bool",
		config:  Config{"param1": "not-a-bool"},
		params:  Parameters{"param1": {Type: ParameterTypeBool}},
		wantErr: true,
	}, {
		name:    "valid type duration",
		config:  Config{"param1": "1s"},
		params:  Parameters{"param1": {Type: ParameterTypeDuration}},
		wantErr: false,
	}, {
		name:    "empty value is valid for all types",
		config:  Config{"param1": ""},
		params:  Parameters{"param1": {Type: ParameterTypeDuration}},
		wantErr: false,
	}, {
		name:    "invalid type duration",
		config:  Config{"param1": "not-a-duration"},
		params:  Parameters{"param1": {Type: ParameterTypeDuration}},
		wantErr: true,
	}, {
		name:    "valid type string",
		config:  Config{"param1": "param"},
		params:  Parameters{"param1": {Type: ParameterTypeString}},
		wantErr: false,
	}, {
		name:    "valid type file",
		config:  Config{"param1": "some-data"},
		params:  Parameters{"param1": {Type: ParameterTypeFile}},
		wantErr: false,
	}, {
		// ---------------------- DYNAMIC PARAMETER TESTS ----------------------
		name:    "dynamic: valid type number",
		config:  Config{"foo.0.param1": "3"},
		params:  Parameters{"foo.*.param1": {Default: "3.3", Type: ParameterTypeFloat}},
		wantErr: false,
	}, {
		name:    "dynamic: invalid type float",
		config:  Config{"foo.0.param1": "not-a-number"},
		params:  Parameters{"foo.*.param1": {Default: "3.3", Type: ParameterTypeFloat}},
		wantErr: true,
	}, {
		name:    "dynamic: valid default type float",
		config:  Config{"foo.0.param1": ""},
		params:  Parameters{"foo.*.param1": {Default: "3", Type: ParameterTypeFloat}},
		wantErr: false,
	}, {
		name:    "dynamic: valid type int",
		config:  Config{"foo.0.param1": "3"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeInt}},
		wantErr: false,
	}, {
		name:    "dynamic: invalid type int",
		config:  Config{"foo.0.param1": "3.3"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeInt}},
		wantErr: true,
	}, {
		name:    "dynamic: valid type bool",
		config:  Config{"foo.0.param1": "1"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeBool}},
		wantErr: false,
	}, {
		name:    "dynamic: valid type bool",
		config:  Config{"foo.0.param1": "true"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeBool}},
		wantErr: false,
	}, {
		name:    "dynamic: invalid type bool",
		config:  Config{"foo.0.param1": "not-a-bool"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeBool}},
		wantErr: true,
	}, {
		name:    "dynamic: valid type duration",
		config:  Config{"foo.0.param1": "1s"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeDuration}},
		wantErr: false,
	}, {
		name:    "dynamic: empty value is valid for all types",
		config:  Config{"foo.0.param1": ""},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeDuration}},
		wantErr: false,
	}, {
		name:    "dynamic: invalid type duration",
		config:  Config{"foo.0.param1": "not-a-duration"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeDuration}},
		wantErr: true,
	}, {
		name:    "dynamic: valid type string",
		config:  Config{"foo.0.param1": "param"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeString}},
		wantErr: false,
	}, {
		name:    "dynamic: valid type file",
		config:  Config{"foo.0.param1": "some-data"},
		params:  Parameters{"foo.*.param1": {Type: ParameterTypeFile}},
		wantErr: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := is.New(t)
			err := tt.config.Sanitize().
				ApplyDefaults(tt.params).
				Validate(tt.params)

			if err != nil && tt.wantErr {
				is.True(errors.Is(err, ErrInvalidParameterType))
			} else if err != nil || tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_Validations(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		params  Parameters
		wantErr bool
		err     error
	}{{
		name:   "required validation failed",
		config: Config{"param1": ""},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
			}},
		},
		wantErr: true,
		err:     ErrRequiredParameterMissing,
	}, {
		name:   "required validation pass",
		config: Config{"param1": "value"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
			}},
		},
		wantErr: false,
	}, {
		name:   "less than validation failed",
		config: Config{"param1": "20"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationLessThan{10},
			}},
		},
		wantErr: true,
		err:     ErrLessThanValidationFail,
	}, {
		name:   "less than validation pass",
		config: Config{"param1": "0"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationLessThan{10},
			}},
		},
		wantErr: false,
	}, {
		name:   "greater than validation failed",
		config: Config{"param1": "0"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationGreaterThan{10},
			}},
		},
		wantErr: true,
		err:     ErrGreaterThanValidationFail,
	}, {
		name:   "greater than validation pass",
		config: Config{"param1": "20"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationGreaterThan{10},
			}},
		},
		wantErr: false,
	}, {
		name:   "inclusion validation failed",
		config: Config{"param1": "three"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationInclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: true,
		err:     ErrInclusionValidationFail,
	}, {
		name:   "inclusion validation pass",
		config: Config{"param1": "two"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationInclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: false,
	}, {
		name:   "exclusion validation failed",
		config: Config{"param1": "one"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationExclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: true,
		err:     ErrExclusionValidationFail,
	}, {
		name:   "exclusion validation pass",
		config: Config{"param1": "three"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationExclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: false,
	}, {
		name:   "regex validation failed",
		config: Config{"param1": "a-a"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationRegex{regexp.MustCompile("[a-z]-[1-9]")},
			}},
		},
		wantErr: true,
		err:     ErrRegexValidationFail,
	}, {
		name:   "regex validation pass",
		config: Config{"param1": "a-8"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationRegex{regexp.MustCompile("[a-z]-[1-9]")},
			}},
		},
		wantErr: false,
	}, {
		name:   "optional validation pass",
		config: Config{"param1": ""},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationInclusion{[]string{"one", "two"}},
				ValidationExclusion{[]string{"three", "four"}},
				ValidationRegex{regexp.MustCompile("[a-z]")},
				ValidationGreaterThan{10},
				ValidationLessThan{20},
			}},
		},
		wantErr: false,
	}, {
		// ---------------------- DYNAMIC PARAMETER TESTS ----------------------
		name:   "dynamic: required validation failed",
		config: Config{"foo.0.param1": ""},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
			}},
		},
		wantErr: true,
		err:     ErrRequiredParameterMissing,
	}, {
		name:   "dynamic: required validation pass",
		config: Config{"foo.0.param1": "value"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
			}},
		},
		wantErr: false,
	}, {
		name:   "dynamic: less than validation failed",
		config: Config{"foo.0.param1": "20"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationLessThan{10},
			}},
		},
		wantErr: true,
		err:     ErrLessThanValidationFail,
	}, {
		name:   "dynamic: less than validation pass",
		config: Config{"foo.0.param1": "0"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationLessThan{10},
			}},
		},
		wantErr: false,
	}, {
		name:   "dynamic: greater than validation failed",
		config: Config{"foo.0.param1": "0"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationGreaterThan{10},
			}},
		},
		wantErr: true,
		err:     ErrGreaterThanValidationFail,
	}, {
		name:   "dynamic: greater than validation pass",
		config: Config{"foo.0.param1": "20"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationGreaterThan{10},
			}},
		},
		wantErr: false,
	}, {
		name:   "dynamic: inclusion validation failed",
		config: Config{"foo.0.param1": "three"},
		params: Parameters{
			"param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationInclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: true,
		err:     ErrInclusionValidationFail,
	}, {
		name:   "dynamic: inclusion validation pass",
		config: Config{"foo.0.param1": "two"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationInclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: false,
	}, {
		name:   "dynamic: exclusion validation failed",
		config: Config{"foo.0.param1": "one"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationExclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: true,
		err:     ErrExclusionValidationFail,
	}, {
		name:   "dynamic: exclusion validation pass",
		config: Config{"foo.0.param1": "three"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationExclusion{[]string{"one", "two"}},
			}},
		},
		wantErr: false,
	}, {
		name:   "dynamic: regex validation failed",
		config: Config{"foo.0.param1": "a-a"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationRegex{regexp.MustCompile("[a-z]-[1-9]")},
			}},
		},
		wantErr: true,
		err:     ErrRegexValidationFail,
	}, {
		name:   "dynamic: regex validation pass",
		config: Config{"foo.0.param1": "a-8"},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationRequired{},
				ValidationRegex{regexp.MustCompile("[a-z]-[1-9]")},
			}},
		},
		wantErr: false,
	}, {
		name:   "dynamic: optional validation pass",
		config: Config{"foo.0.param1": ""},
		params: Parameters{
			"foo.*.param1": {Validations: []Validation{
				ValidationInclusion{[]string{"one", "two"}},
				ValidationExclusion{[]string{"three", "four"}},
				ValidationRegex{regexp.MustCompile("[a-z]")},
				ValidationGreaterThan{10},
				ValidationLessThan{20},
			}},
		},
		wantErr: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := is.New(t)
			err := tt.config.Sanitize().
				ApplyDefaults(tt.params).
				Validate(tt.params)

			if err != nil && tt.wantErr {
				is.True(errors.Is(err, tt.err))
			} else if err != nil || tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_Unrecognized(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		params  Parameters
		wantErr bool
	}{{
		name:    "parameters empty",
		config:  Config{"param1": "3"},
		params:  Parameters{},
		wantErr: true,
	}, {
		name:    "static parameter unrecognized",
		config:  Config{"param1": "not-a-number"},
		params:  Parameters{"param2": {Type: ParameterTypeFloat}},
		wantErr: true,
	}, {
		name:    "dynamic parameter unrecognized",
		config:  Config{"foo.0.param1.": ""},
		params:  Parameters{"foo.*.param2": {Type: ParameterTypeFloat}},
		wantErr: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := is.New(t)
			err := tt.config.Sanitize().
				ApplyDefaults(tt.params).
				Validate(tt.params)

			if err != nil && tt.wantErr {
				is.True(errors.Is(err, ErrUnrecognizedParameter))
			} else if err != nil || tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_MultiError(t *testing.T) {
	is := is.New(t)

	params := map[string]Parameter{
		"limit": {
			Type: ParameterTypeInt,
			Validations: []Validation{
				ValidationGreaterThan{0},
				ValidationRegex{regexp.MustCompile("^[0-9]")},
			},
		},
		"option": {
			Type: ParameterTypeString,
			Validations: []Validation{
				ValidationInclusion{[]string{"one", "two", "three"}},
				ValidationExclusion{[]string{"one", "five"}},
			},
		},
		"name": {
			Type: ParameterTypeString,
			Validations: []Validation{
				ValidationRequired{},
			},
		},
	}
	cfg := Config{
		"limit":  "-1",
		"option": "five",
	}

	err := cfg.Sanitize().
		ApplyDefaults(params).
		Validate(params)
	is.True(err != nil)

	errs := unwrapErrors(err)
	want := []error{
		ErrRequiredParameterMissing,
		ErrInclusionValidationFail,
		ErrExclusionValidationFail,
		ErrGreaterThanValidationFail,
		ErrRegexValidationFail,
	}

OUTER:
	for _, gotErr := range errs {
		for j, wantErr := range want {
			if errors.Is(gotErr, wantErr) {
				// remove error from want and continue asserting the rest
				want = append(want[:j], want[j+1:]...)
				continue OUTER
			}
		}
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if len(want) != 0 {
		t.Fatalf("expected more errors: %v", want)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	params := map[string]Parameter{
		"limit":        {Type: ParameterTypeInt, Default: "1"},
		"foo.*.param1": {Type: ParameterTypeString, Default: "foo"},
		"foo.*.param2": {Type: ParameterTypeString},
	}

	testCases := []struct {
		name string
		have Config
		want Config
	}{{
		name: "empty",
		have: Config{},
		want: Config{
			"limit": "1",
		},
	}, {
		name: "foo.0.param2",
		have: Config{
			"foo.0.param2": "bar",
		},
		want: Config{
			"limit":        "1",
			"foo.0.param1": "foo",
			"foo.0.param2": "bar",
		},
	}, {
		name: "foo.0.param1",
		have: Config{
			"limit":        "-1",
			"foo.0.param1": "custom",
		},
		want: Config{
			"limit":        "-1",
			"foo.0.param1": "custom",
			"foo.0.param2": "",
		},
	}, {
		name: "multiple dynamic params",
		have: Config{
			"limit":                "-1",
			"foo.0.param1":         "parameter",
			"foo.1.param2":         "custom",
			"foo.2.does-not-exist": "unrecognized key still triggers creation of defaults",
		},
		want: Config{
			"limit":                "-1",
			"foo.0.param1":         "parameter",
			"foo.0.param2":         "",
			"foo.1.param1":         "foo",
			"foo.1.param2":         "custom",
			"foo.2.param1":         "foo",
			"foo.2.param2":         "",
			"foo.2.does-not-exist": "unrecognized key still triggers creation of defaults",
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			got := tc.have.Sanitize().
				ApplyDefaults(params)
			is.Equal(tc.want, got)
		})
	}
}

// unwrapErrors recursively unwraps all errors combined using errors.Join.
func unwrapErrors(err error) []error {
	errs, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}
	// unwrap recursively all sub-errors
	var out []error
	for _, err := range errs.Unwrap() {
		out = append(out, unwrapErrors(err)...)
	}
	return out
}

func TestParseConfig_Simple_Struct(t *testing.T) {
	is := is.New(t)

	type Person struct {
		Name string `json:"person_name"`
		Age  int
		Dur  time.Duration
	}

	input := Config{
		"person_name": "meroxa",
		"age":         "91",
		"dur":         "", // empty value should result in zero value
	}
	want := Person{
		Name: "meroxa",
		Age:  91,
	}

	var got Person
	err := input.DecodeInto(&got)
	is.NoErr(err)
	is.Equal(want, got)
}

func TestParseConfig_Embedded_Struct(t *testing.T) {
	is := is.New(t)

	type Family struct {
		LastName string `json:"last.name"`
	}
	type Location struct {
		City string
	}
	type Person struct {
		Family          // last.name
		Location        // City
		F1       Family // F1.last.name
		// City
		L1        Location `json:",squash"` //nolint:staticcheck // json here is a rename for the mapstructure tag
		L2        Location // L2.City
		L3        Location `json:"loc3"`       // loc3.City
		FirstName string   `json:"First.Name"` // First.Name
		First     string   // First
	}

	input := Config{
		"last.name":    "meroxa",
		"F1.last.name": "turbine",
		"City":         "San Francisco",
		"L2.City":      "Paris",
		"loc3.City":    "London",
		"First.Name":   "conduit",
		"First":        "Mickey",
	}
	want := Person{
		Family:    Family{LastName: "meroxa"},
		F1:        Family{LastName: "turbine"},
		Location:  Location{City: "San Francisco"},
		L1:        Location{City: "San Francisco"},
		L2:        Location{City: "Paris"},
		L3:        Location{City: "London"},
		FirstName: "conduit",
		First:     "Mickey",
	}

	var got Person
	err := input.DecodeInto(&got)
	is.NoErr(err)
	is.Equal(want, got)
}

func TestParseConfig_All_Types(t *testing.T) {
	is := is.New(t)
	type structMapVal struct {
		MyString string
		MyInt    int
	}
	type testCfg struct {
		MyString      string
		MyBool1       bool
		MyBool2       bool
		MyBool3       bool
		MyBoolDefault bool

		MyInt    int
		MyUint   uint
		MyInt8   int8
		MyUint8  uint8
		MyInt16  int16
		MyUint16 uint16
		MyInt32  int32
		MyUint32 uint32
		MyInt64  int64
		MyUint64 uint64

		MyByte byte
		MyRune rune

		MyFloat32 float32
		MyFloat64 float64

		MyDuration        time.Duration
		MyDurationDefault time.Duration

		MySlice      []string
		MyIntSlice   []int
		MyFloatSlice []float32

		Nested struct {
			MyString string
		}
		StringMap map[string]string
		StructMap map[string]structMapVal
	}

	input := Config{
		"mystring": "string",
		"mybool1":  "t",
		"mybool2":  "true",
		"mybool3":  "1", // 1 is true
		"myInt":    "-1",
		"myuint":   "1",
		"myint8":   "-1",
		"myuint8":  "1",
		"myInt16":  "-1",
		"myUint16": "1",
		"myint32":  "-1",
		"myuint32": "1",
		"myint64":  "-1",
		"myuint64": "1",

		"mybyte": "99", // 99 fits in one byte
		"myrune": "4567",

		"myfloat32": "1.1122334455",
		"myfloat64": "1.1122334455",

		"myduration": "1s",

		"myslice":      "1,2,3,4",
		"myIntSlice":   "1,2,3,4",
		"myFloatSlice": "1.1,2.2",

		"nested.mystring": "string",

		"stringmap.foo":     "1",
		"stringmap.bar":     "2",
		"stringmap.baz.qux": "3",

		"structmap.foo.mystring": "foo-name",
		"structmap.foo.myint":    "1",
		"structmap.bar.mystring": "bar-name",
		"structmap.bar.myint":    "-1",
	}
	want := testCfg{
		MyString:          "string",
		MyBool1:           true,
		MyBool2:           true,
		MyBool3:           true,
		MyBoolDefault:     false, // default
		MyInt:             -1,
		MyUint:            0x1,
		MyInt8:            -1,
		MyUint8:           0x1,
		MyInt16:           -1,
		MyUint16:          0x1,
		MyInt32:           -1,
		MyUint32:          0x1,
		MyInt64:           -1,
		MyUint64:          0x1,
		MyByte:            0x63,
		MyRune:            4567,
		MyFloat32:         1.1122334,
		MyFloat64:         1.1122334455,
		MyDuration:        1000000000,
		MyDurationDefault: 0,
		MySlice:           []string{"1", "2", "3", "4"},
		MyIntSlice:        []int{1, 2, 3, 4},
		MyFloatSlice:      []float32{1.1, 2.2},
		Nested:            struct{ MyString string }{MyString: "string"},
		StringMap: map[string]string{
			"foo":     "1",
			"bar":     "2",
			"baz.qux": "3",
		},
		StructMap: map[string]structMapVal{
			"foo": {MyString: "foo-name", MyInt: 1},
			"bar": {MyString: "bar-name", MyInt: -1},
		},
	}

	var result testCfg
	err := input.DecodeInto(&result)
	is.NoErr(err)
	is.Equal(want, result)
}

func TestBreakUpConfig(t *testing.T) {
	is := is.New(t)

	input := Config{
		"foo.bar.baz": "1",
		"test":        "2",
	}
	want := map[string]interface{}{
		"foo": map[string]interface{}{
			"bar": map[string]interface{}{
				"baz": "1",
			},
			"bar.baz": "1",
		},
		"foo.bar.baz": "1",
		"test":        "2",
	}
	got := input.breakUp()
	is.Equal(want, got)
}

func TestBreakUpConfig_Conflict_Value(t *testing.T) {
	is := is.New(t)

	input := Config{
		"foo":         "1",
		"foo.bar.baz": "1", // key foo is already taken, will not be broken up
	}
	want := map[string]interface{}{
		"foo":         "1",
		"foo.bar.baz": "1",
	}
	got := input.breakUp()
	is.Equal(want, got)
}

func TestConfig_getValuesForParameter(t *testing.T) {
	cfg := Config{
		"ignore":          "me",
		"ignore.foo.this": "me",

		// foo
		"test.foo.val": "0",

		"test.foo.format.baz.type":    "0",
		"test.foo.format.baz.options": "0",

		"test.foo.format.qux.type":    "0",
		"test.foo.format.qux.options": "0",

		// bar
		"test.bar.val": "0",

		"test.bar.format.baz.type":    "0",
		"test.bar.format.baz.options": "0",

		"test.bar.format.qux.type":    "0",
		"test.bar.format.qux.options": "0",

		// include
		"test.include.me": "yes",

		// ignore this, it's not nested
		"test.ignore": "0",
	}

	testCases := []struct {
		key  string
		want []string
	}{{
		key:  "test.foo.val",
		want: []string{"test.foo.val"},
	}, {
		key:  "blah",
		want: []string{"blah"},
	}, {
		key: "test.*.blah",
		want: []string{
			// Note that the function returns keys that don't exist in the config,
			// it figures out the potential keys based on matched wildcards.
			// However, it does not return test.ignore.blah, as test.ignore does
			// not contain any nested keys.
			"test.foo.blah",
			"test.bar.blah",
			"test.include.blah",
		},
	}, {
		key: "test.*",
		want: []string{
			"test.foo.val",
			"test.foo.format.baz.type",
			"test.foo.format.baz.options",
			"test.foo.format.qux.type",
			"test.foo.format.qux.options",
			"test.bar.val",
			"test.bar.format.baz.type",
			"test.bar.format.baz.options",
			"test.bar.format.qux.type",
			"test.bar.format.qux.options",
			"test.include.me",
			"test.ignore",
		},
	}, {
		key: "test.*.val",
		want: []string{
			"test.foo.val",
			"test.bar.val",
			"test.include.val",
		},
	}, {
		key: "test.*.format.*",
		want: []string{
			"test.foo.format.baz.type",
			"test.foo.format.baz.options",
			"test.foo.format.qux.type",
			"test.foo.format.qux.options",
			"test.bar.format.baz.type",
			"test.bar.format.baz.options",
			"test.bar.format.qux.type",
			"test.bar.format.qux.options",
		},
	}, {
		key: "test.*.format.*.type",
		want: []string{
			"test.foo.format.baz.type",
			"test.foo.format.qux.type",
			"test.bar.format.baz.type",
			"test.bar.format.qux.type",
		},
	}, {
		key: "test.*.format.*.options",
		want: []string{
			"test.foo.format.baz.options",
			"test.foo.format.qux.options",
			"test.bar.format.baz.options",
			"test.bar.format.qux.options",
		},
	}, {
		key: "*",
		want: []string{
			"ignore",
			"ignore.foo.this",
			"test.foo.val",
			"test.foo.format.baz.type",
			"test.foo.format.baz.options",
			"test.foo.format.qux.type",
			"test.foo.format.qux.options",
			"test.bar.val",
			"test.bar.format.baz.type",
			"test.bar.format.baz.options",
			"test.bar.format.qux.type",
			"test.bar.format.qux.options",
			"test.include.me",
			"test.ignore",
		},
	}, {
		key: "*.foo.*",
		want: []string{
			"ignore.foo.this",
			"test.foo.val",
			"test.foo.format.baz.type",
			"test.foo.format.baz.options",
			"test.foo.format.qux.type",
			"test.foo.format.qux.options",
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			is := is.New(t)
			got := cfg.getKeysForParameter(tc.key)

			sort.Strings(tc.want)
			sort.Strings(got)
			is.Equal(cmp.Diff(tc.want, got), "")
		})
	}
}
