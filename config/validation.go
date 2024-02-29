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

//go:generate stringer -type=ValidationType -linecomment

package config

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
)

type Validation interface {
	Type() ValidationType
	Value() string

	Validate(string) error
}

type ValidationType int64

const (
	ValidationTypeRequired    ValidationType = iota + 1 // required
	ValidationTypeGreaterThan                           // greater-than
	ValidationTypeLessThan                              // less-than
	ValidationTypeInclusion                             // inclusion
	ValidationTypeExclusion                             // exclusion
	ValidationTypeRegex                                 // regex
)

func (vt ValidationType) MarshalText() ([]byte, error) {
	return []byte(vt.String()), nil
}

type ValidationRequired struct{}

func (v ValidationRequired) Type() ValidationType { return ValidationTypeRequired }
func (v ValidationRequired) Value() string        { return "" }
func (v ValidationRequired) Validate(value string) error {
	if value == "" {
		return ErrRequiredParameterMissing
	}
	return nil
}
func (v ValidationRequired) MarshalJSON() ([]byte, error) { return jsonMarshalValidation(v) }

type ValidationGreaterThan struct {
	V float64
}

func (v ValidationGreaterThan) Type() ValidationType { return ValidationTypeGreaterThan }
func (v ValidationGreaterThan) Value() string        { return strconv.FormatFloat(v.V, 'f', -1, 64) }
func (v ValidationGreaterThan) Validate(value string) error {
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%q value should be a number: %w", value, ErrInvalidParameterValue)
	}
	if !(val > v.V) {
		formatted := strconv.FormatFloat(v.V, 'f', -1, 64)
		return fmt.Errorf("%q should be greater than %s: %w", value, formatted, ErrGreaterThanValidationFail)
	}
	return nil
}
func (v ValidationGreaterThan) MarshalJSON() ([]byte, error) { return jsonMarshalValidation(v) }

type ValidationLessThan struct {
	V float64
}

func (v ValidationLessThan) Type() ValidationType { return ValidationTypeLessThan }
func (v ValidationLessThan) Value() string        { return strconv.FormatFloat(v.V, 'f', -1, 64) }
func (v ValidationLessThan) Validate(value string) error {
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%q value should be a number: %w", value, ErrInvalidParameterValue)
	}
	if !(val < v.V) {
		formatted := strconv.FormatFloat(v.V, 'f', -1, 64)
		return fmt.Errorf("%q should be less than %s: %w", value, formatted, ErrLessThanValidationFail)
	}
	return nil
}
func (v ValidationLessThan) MarshalJSON() ([]byte, error) { return jsonMarshalValidation(v) }

type ValidationInclusion struct {
	List []string
}

func (v ValidationInclusion) Type() ValidationType { return ValidationTypeInclusion }
func (v ValidationInclusion) Value() string        { return strings.Join(v.List, ",") }
func (v ValidationInclusion) Validate(value string) error {
	if !slices.Contains(v.List, value) {
		return fmt.Errorf("%q value must be included in the list [%s]: %w", value, strings.Join(v.List, ","), ErrInclusionValidationFail)
	}
	return nil
}
func (v ValidationInclusion) MarshalJSON() ([]byte, error) { return jsonMarshalValidation(v) }

type ValidationExclusion struct {
	List []string
}

func (v ValidationExclusion) Type() ValidationType { return ValidationTypeExclusion }
func (v ValidationExclusion) Value() string        { return strings.Join(v.List, ",") }
func (v ValidationExclusion) Validate(value string) error {
	if slices.Contains(v.List, value) {
		return fmt.Errorf("%q value must be excluded from the list [%s]: %w", value, strings.Join(v.List, ","), ErrExclusionValidationFail)
	}
	return nil
}
func (v ValidationExclusion) MarshalJSON() ([]byte, error) { return jsonMarshalValidation(v) }

type ValidationRegex struct {
	Regex *regexp.Regexp
}

func (v ValidationRegex) Type() ValidationType { return ValidationTypeRegex }
func (v ValidationRegex) Value() string        { return v.Regex.String() }
func (v ValidationRegex) Validate(value string) error {
	if !v.Regex.MatchString(value) {
		return fmt.Errorf("%q should match the regex %q: %w", value, v.Regex.String(), ErrRegexValidationFail)
	}
	return nil
}
func (v ValidationRegex) MarshalJSON() ([]byte, error) { return jsonMarshalValidation(v) }

func jsonMarshalValidation(v Validation) ([]byte, error) {
	//nolint:wrapcheck // no need to wrap this error, this will be called by the JSON lib itself
	return json.Marshal(map[string]any{
		"type":  v.Type(),
		"value": v.Value(),
	})
}
