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
	"regexp"
	"testing"

	parameterv1 "github.com/conduitio/conduit-commons/proto/parameter/v1"
	"github.com/matryer/is"
)

func TestParameter_FromProto(t *testing.T) {
	is := is.New(t)

	have := &parameterv1.Parameter{
		Description: "test-description",
		Default:     "test-default",
		Type:        parameterv1.Parameter_TYPE_STRING,
		Validations: []*parameterv1.Validation{
			{Type: parameterv1.Validation_TYPE_REQUIRED},
			{Type: parameterv1.Validation_TYPE_GREATER_THAN, Value: "1.2"},
			{Type: parameterv1.Validation_TYPE_LESS_THAN, Value: "3.4"},
			{Type: parameterv1.Validation_TYPE_INCLUSION, Value: "1,2,3"},
			{Type: parameterv1.Validation_TYPE_EXCLUSION, Value: "4,5,6"},
			{Type: parameterv1.Validation_TYPE_REGEX, Value: "test-regex"},
		},
	}

	want := Parameter{
		Description: "test-description",
		Default:     "test-default",
		Type:        ParameterTypeString,
		Validations: []Validation{
			ValidationRequired{},
			ValidationGreaterThan{V: 1.2},
			ValidationLessThan{V: 3.4},
			ValidationInclusion{List: []string{"1", "2", "3"}},
			ValidationExclusion{List: []string{"4", "5", "6"}},
			ValidationRegex{Regex: regexp.MustCompile("test-regex")},
		},
	}

	var got Parameter
	err := got.FromProto(have)
	is.NoErr(err)
	is.Equal(want, got)
}

func TestParameter_ToProto(t *testing.T) {
	is := is.New(t)

	have := Parameter{
		Description: "test-description",
		Default:     "test-default",
		Type:        ParameterTypeString,
		Validations: []Validation{
			ValidationRequired{},
			ValidationRegex{Regex: regexp.MustCompile("test-regex")},
		},
	}

	want := &parameterv1.Parameter{
		Description: "test-description",
		Default:     "test-default",
		Type:        parameterv1.Parameter_TYPE_STRING,
		Validations: []*parameterv1.Validation{
			{Type: parameterv1.Validation_TYPE_REQUIRED},
			{Type: parameterv1.Validation_TYPE_REGEX, Value: "test-regex"},
		},
	}

	got := &parameterv1.Parameter{}
	have.ToProto(got)
	is.Equal(want, got)
}

func TestParameter_ParameterTypes(t *testing.T) {
	is := is.New(t)

	testCases := []struct {
		protoType parameterv1.Parameter_Type
		goType    ParameterType
	}{
		{protoType: parameterv1.Parameter_TYPE_UNSPECIFIED, goType: 0},
		{protoType: parameterv1.Parameter_TYPE_STRING, goType: ParameterTypeString},
		{protoType: parameterv1.Parameter_TYPE_INT, goType: ParameterTypeInt},
		{protoType: parameterv1.Parameter_TYPE_FLOAT, goType: ParameterTypeFloat},
		{protoType: parameterv1.Parameter_TYPE_BOOL, goType: ParameterTypeBool},
		{protoType: parameterv1.Parameter_TYPE_FILE, goType: ParameterTypeFile},
		{protoType: parameterv1.Parameter_TYPE_DURATION, goType: ParameterTypeDuration},
		{protoType: parameterv1.Parameter_Type(100), goType: 100},
	}

	t.Run("FromProto", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.goType.String(), func(t *testing.T) {
				have := &parameterv1.Parameter{Type: tc.protoType}
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
				have := Parameter{Type: tc.goType}
				want := &parameterv1.Parameter{Type: tc.protoType}

				got := &parameterv1.Parameter{}
				have.ToProto(got)
				is.Equal(want, got)
			})
		}
	})
}

func TestParameter_Validation(t *testing.T) {
	is := is.New(t)

	testCases := []struct {
		protoType *parameterv1.Validation
		goType    Validation
	}{
		{
			protoType: &parameterv1.Validation{Type: parameterv1.Validation_TYPE_REQUIRED},
			goType:    ValidationRequired{},
		},
		{
			protoType: &parameterv1.Validation{Type: parameterv1.Validation_TYPE_GREATER_THAN, Value: "1.2"},
			goType:    ValidationGreaterThan{V: 1.2},
		},
		{
			protoType: &parameterv1.Validation{Type: parameterv1.Validation_TYPE_LESS_THAN, Value: "3.4"},
			goType:    ValidationLessThan{V: 3.4},
		},
		{
			protoType: &parameterv1.Validation{Type: parameterv1.Validation_TYPE_INCLUSION, Value: "1,2,3"},
			goType:    ValidationInclusion{List: []string{"1", "2", "3"}},
		},
		{
			protoType: &parameterv1.Validation{Type: parameterv1.Validation_TYPE_EXCLUSION, Value: "4,5,6"},
			goType:    ValidationExclusion{List: []string{"4", "5", "6"}},
		},
		{
			protoType: &parameterv1.Validation{Type: parameterv1.Validation_TYPE_REGEX, Value: "test-regex"},
			goType:    ValidationRegex{Regex: regexp.MustCompile("test-regex")},
		},
	}

	t.Run("FromProto", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.goType.Type().String(), func(t *testing.T) {
				have := &parameterv1.Parameter{
					Validations: []*parameterv1.Validation{tc.protoType},
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
				have := Parameter{
					Validations: []Validation{tc.goType},
				}
				want := &parameterv1.Parameter{
					Validations: []*parameterv1.Validation{tc.protoType},
				}

				got := &parameterv1.Parameter{}
				have.ToProto(got)
				is.Equal(want, got)
			})
		}
	})
}
