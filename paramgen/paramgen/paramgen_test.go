// Copyright © 2023 Meroxa, Inc.
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

package paramgen

import (
	"errors"
	"regexp"
	"testing"

	"github.com/conduitio/conduit-commons/config"
	"github.com/matryer/is"
)

func TestParseParametersSuccess(t *testing.T) {
	testCases := []struct {
		path string
		name string
		pkg  string
		want map[string]config.Parameter
	}{
		{
			path: "./testdata/basic",
			name: "SourceConfig",
			pkg:  "example",
			want: map[string]config.Parameter{
				"foo": {
					Default:     "bar",
					Description: "MyGlobalString is a required field in the global config with the name\n\"foo\" and default value \"bar\".",
					Type:        config.ParameterTypeString,
					Validations: []config.Validation{
						config.ValidationRequired{},
					},
				},
				"myString": {
					Description: "MyString my string description",
					Type:        config.ParameterTypeString,
				},
				"myBool": {Type: config.ParameterTypeBool},
				"myInt": {
					Type: config.ParameterTypeInt,
					Validations: []config.Validation{
						config.ValidationLessThan{V: 100},
						config.ValidationGreaterThan{V: 0},
					},
				},
				"myUint": {
					Type: config.ParameterTypeInt,
					Validations: []config.Validation{
						config.ValidationLessThanOrEqual{V: 101},
						config.ValidationGreaterThanOrEqual{V: 1},
					},
				},
				"myInt8":                 {Type: config.ParameterTypeInt},
				"myUint8":                {Type: config.ParameterTypeInt},
				"myInt16":                {Type: config.ParameterTypeInt},
				"myUint16":               {Type: config.ParameterTypeInt},
				"myInt32":                {Type: config.ParameterTypeInt},
				"myUint32":               {Type: config.ParameterTypeInt},
				"myInt64":                {Type: config.ParameterTypeInt},
				"myUint64":               {Type: config.ParameterTypeInt},
				"myByte":                 {Type: config.ParameterTypeString},
				"myRune":                 {Type: config.ParameterTypeInt},
				"myFloat32":              {Type: config.ParameterTypeFloat},
				"myFloat64":              {Type: config.ParameterTypeFloat},
				"myDuration":             {Type: config.ParameterTypeDuration},
				"myIntSlice":             {Type: config.ParameterTypeString},
				"myFloatSlice":           {Type: config.ParameterTypeString},
				"myDurSlice":             {Type: config.ParameterTypeString},
				"myStringMap.*":          {Type: config.ParameterTypeString},
				"myStructMap.*.myInt":    {Type: config.ParameterTypeInt},
				"myStructMap.*.myString": {Type: config.ParameterTypeString},
				"myBoolPtr":              {Type: config.ParameterTypeBool},
				"myDurationPtr":          {Type: config.ParameterTypeDuration},
			},
		},
		{
			path: "./testdata/complex",
			name: "SourceConfig",
			pkg:  "example",
			want: map[string]config.Parameter{
				"global.duration": {
					Default:     "1s",
					Description: "Duration does not have a name so the type name is used.",
					Type:        config.ParameterTypeDuration,
				},
				"global.wildcardStrings.*": {
					Default: "foo",
					Type:    config.ParameterTypeString,
					Validations: []config.Validation{
						config.ValidationRequired{},
					},
				},
				"global.renamed.*": {
					Default: "1s",
					Type:    config.ParameterTypeDuration,
				},
				"global.wildcardStructs.*.name": {
					Type: config.ParameterTypeString,
				},
				"nestMeHere.anotherNested": {
					Type:        config.ParameterTypeInt,
					Description: "AnotherNested is also nested under nestMeHere.\nThis is a block comment.",
				},
				"nestMeHere.formatThisName": {
					Type:        config.ParameterTypeFloat,
					Default:     "this is not a float",
					Description: "FORMATThisName should stay \"FORMATThisName\". Default is not a float\nbut that's not a problem, paramgen does not validate correctness.",
				},
				"customType": {
					Type:        config.ParameterTypeDuration,
					Description: "CustomType uses a custom type that is convertible to a supported type. Line comments are allowed.",
				},
			},
		},
		{
			path: "./testdata/tags",
			name: "Config",
			pkg:  "tags",
			want: map[string]config.Parameter{
				"my-name": {
					Type:        config.ParameterTypeString,
					Validations: []config.Validation{config.ValidationRequired{}},
				},
				"my-param": {
					Type:        config.ParameterTypeInt,
					Description: "Param1 i am a parameter comment",
					Default:     "3",
					Validations: []config.Validation{
						config.ValidationRequired{},
						config.ValidationGreaterThan{V: 0},
						config.ValidationLessThan{V: 100},
					},
				},
				"param2": {
					Type:    config.ParameterTypeBool,
					Default: "t",
					Validations: []config.Validation{
						config.ValidationInclusion{List: []string{"true", "t"}},
						config.ValidationExclusion{List: []string{"false", "f"}},
					},
				},
				"param3": {
					Type:    config.ParameterTypeString,
					Default: "yes",
					Validations: []config.Validation{
						config.ValidationRequired{},
						config.ValidationRegex{Regex: regexp.MustCompile(".*")},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			is := is.New(t)
			got, pkg, err := ParseParameters(tc.path, tc.name)
			is.NoErr(err)
			is.Equal(pkg, tc.pkg)
			is.Equal(got, tc.want)
		})
	}
}

func TestParseParametersFail(t *testing.T) {
	testCases := []struct {
		path    string
		name    string
		wantErr error
	}{{
		path:    "./testdata/invalid1",
		name:    "SourceConfig",
		wantErr: errors.New("error parsing type spec for http.RoundTripper.Transport: interface types not supported"),
	}, {
		path:    "./testdata/invalid2",
		name:    "SourceConfig",
		wantErr: errors.New("invalid value for tag validate: invalidValidation=hi"),
	}, {
		path:    "./testdata/basic",
		name:    "SomeConfig",
		wantErr: errors.New("struct \"SomeConfig\" was not found in the package \"example\""),
	}}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			is := is.New(t)
			_, pkg, err := ParseParameters(tc.path, tc.name)
			is.Equal(pkg, "")
			is.True(err != nil)
			for {
				unwrapped := errors.Unwrap(err)
				if unwrapped == nil {
					break
				}
				err = unwrapped
			}
			is.Equal(err, tc.wantErr)
		})
	}
}
