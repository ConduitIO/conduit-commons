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

	"github.com/goccy/go-json"
	"github.com/matryer/is"
)

func TestParameters_MarshalJSON(t *testing.T) {
	is := is.New(t)
	parameters := Parameters{
		"empty": {},
		"string": {
			Default:     "test-string",
			Description: "test-description",
			Type:        ParameterTypeString,
			Validations: []Validation{},
		},
		"int": {
			Default:     "test-string",
			Description: "test-description",
			Type:        ParameterTypeInt,
			Validations: []Validation{},
		},
		"float": {
			Default:     "test-float",
			Description: "test-description",
			Type:        ParameterTypeFloat,
			Validations: []Validation{},
		},
		"bool": {
			Default:     "test-bool",
			Description: "test-description",
			Type:        ParameterTypeBool,
			Validations: []Validation{},
		},
		"file": {
			Default:     "test-file",
			Description: "test-description",
			Type:        ParameterTypeFile,
			Validations: []Validation{},
		},
		"duration": {
			Default:     "test-duration",
			Description: "test-description",
			Type:        ParameterTypeDuration,
			Validations: []Validation{},
		},
		"validations": {
			Validations: []Validation{
				ValidationRequired{},
				ValidationGreaterThan{V: 1.2},
				ValidationLessThan{V: 3.4},
				ValidationInclusion{List: []string{"1", "2", "3"}},
				ValidationExclusion{List: []string{"4", "5", "6"}},
				ValidationRegex{Regex: regexp.MustCompile("test-regex")},
			},
		},
	}

	want := `{
  "bool": {
    "default": "test-bool",
    "description": "test-description",
    "type": "bool",
    "validations": []
  },
  "duration": {
    "default": "test-duration",
    "description": "test-description",
    "type": "duration",
    "validations": []
  },
  "empty": {
    "default": "",
    "description": "",
    "type": "ParameterType(0)",
    "validations": null
  },
  "file": {
    "default": "test-file",
    "description": "test-description",
    "type": "file",
    "validations": []
  },
  "float": {
    "default": "test-float",
    "description": "test-description",
    "type": "float",
    "validations": []
  },
  "int": {
    "default": "test-string",
    "description": "test-description",
    "type": "int",
    "validations": []
  },
  "string": {
    "default": "test-string",
    "description": "test-description",
    "type": "string",
    "validations": []
  },
  "validations": {
    "default": "",
    "description": "",
    "type": "ParameterType(0)",
    "validations": [
      {
        "type": "required",
        "value": ""
      },
      {
        "type": "greater-than",
        "value": "1.2"
      },
      {
        "type": "less-than",
        "value": "3.4"
      },
      {
        "type": "inclusion",
        "value": "1,2,3"
      },
      {
        "type": "exclusion",
        "value": "4,5,6"
      },
      {
        "type": "regex",
        "value": "test-regex"
      }
    ]
  }
}`

	got, err := json.MarshalIndent(parameters, "", "  ")
	is.NoErr(err)
	is.Equal(want, string(got))
}
