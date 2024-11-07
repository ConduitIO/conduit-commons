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

	configv1 "github.com/conduitio/conduit-commons/proto/config/v1"
	"github.com/matryer/is"
)

func TestParameter_ParameterTypes(t *testing.T) {
	testCases := []struct {
		protoType configv1.Parameter_Type
		goType    ParameterType
	}{
		{protoType: configv1.Parameter_TYPE_UNSPECIFIED, goType: 0},
		{protoType: configv1.Parameter_TYPE_STRING, goType: ParameterTypeString},
		{protoType: configv1.Parameter_TYPE_INT, goType: ParameterTypeInt},
		{protoType: configv1.Parameter_TYPE_FLOAT, goType: ParameterTypeFloat},
		{protoType: configv1.Parameter_TYPE_BOOL, goType: ParameterTypeBool},
		{protoType: configv1.Parameter_TYPE_FILE, goType: ParameterTypeFile},
		{protoType: configv1.Parameter_TYPE_DURATION, goType: ParameterTypeDuration},
		{protoType: configv1.Parameter_Type(100), goType: 100},
	}

	t.Run("FromProto", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.goType.String(), func(*testing.T) {
				is := is.New(t)
				have := &configv1.Parameter{Type: tc.protoType}
				want := Parameter{Type: tc.goType}

				var got Parameter
				err := got.FromProto(have)
				is.NoErr(err)
				is.Equal(want, got)
			})
		}
	})

	t.Run("ToProto", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.goType.String(), func(t *testing.T) {
				is := is.New(t)
				have := Parameter{Type: tc.goType}
				want := &configv1.Parameter{Type: tc.protoType}

				got := &configv1.Parameter{}
				have.ToProto(got)
				is.Equal(want, got)
			})
		}
	})
}

func TestParameter_Validation(t *testing.T) {
	testCases := []struct {
		protoType *configv1.Validation
		goType    Validation
	}{
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_REQUIRED},
			goType:    ValidationRequired{},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_GREATER_THAN, Value: "1.2"},
			goType:    ValidationGreaterThan{V: 1.2},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_LESS_THAN, Value: "3.4"},
			goType:    ValidationLessThan{V: 3.4},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_INCLUSION, Value: "1,2,3"},
			goType:    ValidationInclusion{List: []string{"1", "2", "3"}},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_EXCLUSION, Value: "4,5,6"},
			goType:    ValidationExclusion{List: []string{"4", "5", "6"}},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_REGEX, Value: "test-regex"},
			goType:    ValidationRegex{Regex: regexp.MustCompile("test-regex")},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_GREATER_THAN_OR_EQUAL, Value: "1.2"},
			goType:    ValidationGreaterThanOrEqual{V: 1.2},
		},
		{
			protoType: &configv1.Validation{Type: configv1.Validation_TYPE_LESS_THAN_OR_EQUAL, Value: "3.4"},
			goType:    ValidationLessThanOrEqual{V: 3.4},
		},
	}

	t.Run("FromProto", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.goType.Type().String(), func(t *testing.T) {
				is := is.New(t)
				have := &configv1.Parameter{
					Validations: []*configv1.Validation{tc.protoType},
				}
				want := Parameter{
					Validations: []Validation{tc.goType},
				}

				var got Parameter
				err := got.FromProto(have)
				is.NoErr(err)
				is.Equal(want, got)
			})
		}
	})

	t.Run("ToProto", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.goType.Type().String(), func(t *testing.T) {
				is := is.New(t)
				have := Parameter{
					Validations: []Validation{tc.goType},
				}
				want := &configv1.Parameter{
					Validations: []*configv1.Validation{tc.protoType},
				}

				got := &configv1.Parameter{}
				have.ToProto(got)
				is.Equal(want, got)
			})
		}
	})
}

func TestParameter_Validation_InvalidType(t *testing.T) {
	is := is.New(t)
	have := &configv1.Parameter{
		Validations: []*configv1.Validation{
			{Type: configv1.Validation_TYPE_UNSPECIFIED},
		},
	}
	var got Parameter
	err := got.FromProto(have)
	is.True(errors.Is(err, ErrInvalidValidationType))
}
