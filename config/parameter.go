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

import parameterv1 "github.com/conduitio/conduit-commons/proto/parameter/v1"

// Parameters is a map of all configuration parameters.
type Parameters map[string]Parameter

// Parameter defines a single configuration parameter.
type Parameter struct {
	// Description holds a description of the field and how to configure it.
	Description string
	// Default is the default value of the parameter, if any.
	Default string
	// Type defines the parameter data type.
	Type ParameterType
	// Validations list of validations to check for the parameter.
	Validations []Validation
}

type ParameterType int

const (
	ParameterTypeString ParameterType = iota + 1
	ParameterTypeInt
	ParameterTypeFloat
	ParameterTypeBool
	ParameterTypeFile
	ParameterTypeDuration
)

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var cTypes [1]struct{}
	_ = cTypes[int(ParameterTypeString)-int(parameterv1.Parameter_TYPE_STRING)]
	_ = cTypes[int(ParameterTypeInt)-int(parameterv1.Parameter_TYPE_INT)]
	_ = cTypes[int(ParameterTypeFloat)-int(parameterv1.Parameter_TYPE_FLOAT)]
	_ = cTypes[int(ParameterTypeBool)-int(parameterv1.Parameter_TYPE_BOOL)]
	_ = cTypes[int(ParameterTypeFile)-int(parameterv1.Parameter_TYPE_FILE)]
	_ = cTypes[int(ParameterTypeDuration)-int(parameterv1.Parameter_TYPE_DURATION)]
}
