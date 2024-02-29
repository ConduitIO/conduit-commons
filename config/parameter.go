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

//go:generate stringer -type=ParameterType -linecomment

package config

// Parameters is a map of all configuration parameters.
type Parameters map[string]Parameter

// Parameter defines a single configuration parameter.
type Parameter struct {
	// Default is the default value of the parameter, if any.
	Default string `json:"default"`
	// Description holds a description of the field and how to configure it.
	Description string `json:"description"`
	// Type defines the parameter data type.
	Type ParameterType `json:"type"`
	// Validations list of validations to check for the parameter.
	Validations []Validation `json:"validations"`
}

type ParameterType int

const (
	ParameterTypeString   ParameterType = iota + 1 // string
	ParameterTypeInt                               // int
	ParameterTypeFloat                             // float
	ParameterTypeBool                              // bool
	ParameterTypeFile                              // file
	ParameterTypeDuration                          // duration
)

func (pt ParameterType) MarshalText() ([]byte, error) {
	return []byte(pt.String()), nil
}
