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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	parameterv1 "github.com/conduitio/conduit-commons/proto/parameter/v1"
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

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	var cTypes [1]struct{}
	_ = cTypes[int(ValidationTypeRequired)-int(parameterv1.Validation_TYPE_REQUIRED)]
	_ = cTypes[int(ValidationTypeGreaterThan)-int(parameterv1.Validation_TYPE_GREATER_THAN)]
	_ = cTypes[int(ValidationTypeLessThan)-int(parameterv1.Validation_TYPE_LESS_THAN)]
	_ = cTypes[int(ValidationTypeInclusion)-int(parameterv1.Validation_TYPE_INCLUSION)]
	_ = cTypes[int(ValidationTypeExclusion)-int(parameterv1.Validation_TYPE_EXCLUSION)]
	_ = cTypes[int(ValidationTypeRegex)-int(parameterv1.Validation_TYPE_REGEX)]
}

// -- From Proto To Parameter --------------------------------------------------

// FromProto takes data from the supplied proto object and populates the
// receiver. If the proto object is nil, the receiver is set to its zero value.
// If the function returns an error, the receiver could be partially populated.
func (p *Parameters) FromProto(proto map[string]*parameterv1.Parameter) error {
	if proto == nil {
		*p = nil
		return nil
	}

	clear(*p)
	for k, v := range proto {
		var param Parameter
		err := param.FromProto(v)
		if err != nil {
			return fmt.Errorf("error converting parameter: %w", err)
		}
		(*p)[k] = param
	}
	return nil
}

// FromProto takes data from the supplied proto object and populates the
// receiver. If the proto object is nil, the receiver is set to its zero value.
// If the function returns an error, the receiver could be partially populated.
func (p *Parameter) FromProto(proto *parameterv1.Parameter) error {
	if proto == nil {
		*p = Parameter{}
		return nil
	}

	var err error
	p.Validations, err = validationsFromProto(proto.Validations)
	if err != nil {
		return err
	}

	p.Default = proto.Default
	p.Description = proto.Description
	p.Type = ParameterType(proto.Type)
	return nil
}

func validationsFromProto(proto []*parameterv1.Validation) ([]Validation, error) {
	if proto == nil {
		return nil, nil
	}

	validations := make([]Validation, len(proto))
	for i, v := range proto {
		var err error
		validations[i], err = validationFromProto(v)
		if err != nil {
			return nil, fmt.Errorf("error converting validation: %w", err)
		}
	}
	return validations, nil
}

func validationFromProto(proto *parameterv1.Validation) (Validation, error) {
	if proto == nil {
		return nil, nil
	}

	switch proto.Type {
	case parameterv1.Validation_TYPE_REQUIRED:
		return ValidationRequired{}, nil
	case parameterv1.Validation_TYPE_GREATER_THAN:
		v, err := strconv.ParseFloat(proto.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing greater than value: %w", err)
		}
		return ValidationGreaterThan{V: v}, nil
	case parameterv1.Validation_TYPE_LESS_THAN:
		v, err := strconv.ParseFloat(proto.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing less than value: %w", err)
		}
		return ValidationLessThan{V: v}, nil
	case parameterv1.Validation_TYPE_INCLUSION:
		return ValidationInclusion{List: strings.Split(proto.Value, ",")}, nil
	case parameterv1.Validation_TYPE_EXCLUSION:
		return ValidationExclusion{List: strings.Split(proto.Value, ",")}, nil
	case parameterv1.Validation_TYPE_REGEX:
		regex, err := regexp.Compile(proto.Value)
		if err != nil {
			return nil, fmt.Errorf("error compiling regex: %w", err)
		}
		return ValidationRegex{Regex: regex}, nil
	default:
		return nil, fmt.Errorf("unknown validation type: %v", proto.Type)
	}
}

// -- From Parameter To Proto --------------------------------------------------

// ToProto takes data from the receiver and populates the supplied proto object.
func (p Parameters) ToProto(proto map[string]*parameterv1.Parameter) {
	clear(proto)
	for k, param := range p {
		var v parameterv1.Parameter
		param.ToProto(&v)
		proto[k] = &v
	}
}

// ToProto takes data from the receiver and populates the supplied proto object.
func (p Parameter) ToProto(proto *parameterv1.Parameter) {
	proto.Default = p.Default
	proto.Description = p.Description
	proto.Type = parameterv1.Parameter_Type(p.Type)
	proto.Validations = validationsToProto(p.Validations)
}

func validationsToProto(validations []Validation) []*parameterv1.Validation {
	if validations == nil {
		return nil
	}

	proto := make([]*parameterv1.Validation, len(validations))
	for i, v := range validations {
		proto[i] = validationToProto(v)
	}
	return proto
}

func validationToProto(validation Validation) *parameterv1.Validation {
	return &parameterv1.Validation{
		Type:  parameterv1.Validation_Type(validation.Type()),
		Value: validation.Value(),
	}
}
