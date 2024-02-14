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
	"testing"

	"github.com/matryer/is"
)

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
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := is.New(t)
			err := tt.config.Sanitize().
				ApplyDefaults(tt.params).
				Validate(tt.params)

			if err != nil && tt.wantErr {
				is.True(errors.Is(err, ErrInvalidParamType))
			} else if err != nil || tt.wantErr {
				t.Errorf("UtilityFunc() error = %v, wantErr %v", err, tt.wantErr)
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
		name:   "greater than validation failed",
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
				t.Errorf("UtilityFunc() error = %v, wantErr %v", err, tt.wantErr)
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
