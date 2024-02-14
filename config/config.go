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

// Package config provides types for specifying the expected configuration of a
// Conduit plugin (connector or processor). It also provides utilities to
// validate the configuration based on the specifications.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config is a map of configuration values. The keys are the configuration
// parameter names and the values are the configuration parameter values.
type Config map[string]string

// Sanitize removes leading and trailing spaces from all keys and values in the
// configuration.
func (c Config) Sanitize() Config {
	for key, val := range c {
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		c[key] = val
	}
	return c
}

// ApplyDefaults applies the default values defined in the parameter
// specifications to the configuration. If a parameter is not present in the
// configuration, the default value is applied.
func (c Config) ApplyDefaults(params Parameters) Config {
	for key, param := range params {
		if strings.TrimSpace(c[key]) == "" {
			c[key] = param.Default
		}
	}
	return c
}

// Validate is a utility function that applies all the validations defined in
// the parameter specifications. It checks for unrecognized parameters, type
// validations, and value validations. It returns all encountered errors.
func (c Config) Validate(params Parameters) error {
	errs := c.validateUnrecognizedParameters(params)

	for key := range params {
		err := c.validateParamType(key, params[key])
		if err != nil {
			// append error and continue with next parameter
			errs = append(errs, err)
			continue
		}
		err = c.validateParamValue(key, params[key])
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// validateUnrecognizedParameters validates that the config only contains
// parameters specified in the parameters.
func (c Config) validateUnrecognizedParameters(params Parameters) []error {
	var errs []error
	for key := range c {
		if _, ok := params[key]; !ok {
			errs = append(errs, fmt.Errorf("%q: %w", key, ErrUnrecognizedParameter))
		}
	}
	return errs
}

// validateParamType validates that a parameter value is parsable to its assigned type.
func (c Config) validateParamType(key string, param Parameter) error {
	value := c[key]
	// empty value is valid for all types
	if c[key] == "" {
		return nil
	}

	//nolint:exhaustive // type ParameterTypeFile and ParameterTypeString don't need type validations (both are strings or byte slices)
	switch param.Type {
	case ParameterTypeInt:
		_, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("error validating %q: %q value is not an integer: %w", key, value, ErrInvalidParamType)
		}
	case ParameterTypeFloat:
		_, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("error validating %q: %q value is not a float: %w", key, value, ErrInvalidParamType)
		}
	case ParameterTypeDuration:
		_, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("error validating %q: %q value is not a duration: %w", key, value, ErrInvalidParamType)
		}
	case ParameterTypeBool:
		_, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("error validating %q: %q value is not a boolean: %w", key, value, ErrInvalidParamType)
		}
	}
	return nil
}

// validateParamValue validates that a configuration value matches all the
// validations required for the parameter.
func (c Config) validateParamValue(key string, param Parameter) error {
	value := c[key]
	var errs []error

	isRequired := false
	for _, v := range param.Validations {
		if _, ok := v.(ValidationRequired); ok {
			isRequired = true
		}
		err := v.Validate(value)
		if err != nil {
			errs = append(errs, fmt.Errorf("error validating %q: %w", key, err))
			continue
		}
	}
	if value == "" && !isRequired {
		return nil // empty optional parameter is valid
	}

	return errors.Join(errs...)
}
