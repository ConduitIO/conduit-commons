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

import (
	"fmt"
	"strconv"
	"strings"
)

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

func (pt *ParameterType) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return nil // empty string, do nothing
	}

	switch string(b) {
	case ParameterTypeString.String():
		*pt = ParameterTypeString
	case ParameterTypeInt.String():
		*pt = ParameterTypeInt
	case ParameterTypeFloat.String():
		*pt = ParameterTypeFloat
	case ParameterTypeBool.String():
		*pt = ParameterTypeBool
	case ParameterTypeFile.String():
		*pt = ParameterTypeFile
	case ParameterTypeDuration.String():
		*pt = ParameterTypeDuration
	default:
		// it may not be a known parameter type, but we also allow ParameterType(int)
		valIntRaw := strings.TrimSuffix(strings.TrimPrefix(string(b), "ParameterType("), ")")
		valInt, err := strconv.Atoi(valIntRaw)
		if err != nil {
			return fmt.Errorf("parameter type %q: %w", b, ErrInvalidParameterType)
		}
		*pt = ParameterType(valInt)
	}

	return nil
}
