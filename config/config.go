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
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

// Config is a map of configuration values. The keys are the configuration
// parameter names and the values are the configuration parameter values.
type Config map[string]string

// Sanitize removes leading and trailing spaces from all keys and values in the
// configuration, and makes sure it's not nil.
func (c Config) Sanitize() Config {
	if c == nil {
		return map[string]string{}
	}

	for key, val := range c {
		delete(c, key)
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
		for _, key := range c.getKeysForParameter(key) {
			if strings.TrimSpace(c[key]) == "" && param.Default != "" {
				c[key] = param.Default
			}
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
		if _, ok := params[key]; ok {
			// Direct match.
			continue
		}
		// Check if the key is a wildcard key.
		match := false
		for pattern := range params {
			if !strings.Contains(pattern, "*") {
				continue
			}
			// Check if the key matches the wildcard key.
			if c.matchParameterKey(key, pattern) {
				match = true
				break
			}
		}

		if !match {
			errs = append(errs, fmt.Errorf("%q: %w", key, ErrUnrecognizedParameter))
		}
	}
	return errs
}

// validateParamType validates that a parameter value is parsable to its assigned type.
func (c Config) validateParamType(key string, param Parameter) error {
	keys := c.getKeysForParameter(key)

	var errs []error
	for _, k := range keys {
		value := c[k]
		// empty value is valid for all types
		if value == "" {
			continue
		}
		//nolint:exhaustive // type ParameterTypeFile and ParameterTypeString don't need type validations (both are strings or byte slices)
		switch param.Type {
		case ParameterTypeInt:
			_, err := strconv.Atoi(value)
			if err != nil {
				errs = append(errs, fmt.Errorf("error validating %q: %q value is not an integer: %w", k, value, ErrInvalidParameterType))
			}
		case ParameterTypeFloat:
			_, err := strconv.ParseFloat(value, 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("error validating %q: %q value is not a float: %w", k, value, ErrInvalidParameterType))
			}
		case ParameterTypeDuration:
			_, err := time.ParseDuration(value)
			if err != nil {
				errs = append(errs, fmt.Errorf("error validating %q: %q value is not a duration: %w", k, value, ErrInvalidParameterType))
			}
		case ParameterTypeBool:
			_, err := strconv.ParseBool(value)
			if err != nil {
				errs = append(errs, fmt.Errorf("error validating %q: %q value is not a boolean: %w", k, value, ErrInvalidParameterType))
			}
		}
	}
	return errors.Join(errs...)
}

// validateParamValue validates that a configuration value matches all the
// validations required for the parameter.
func (c Config) validateParamValue(key string, param Parameter) error {
	var errs []error
	for _, k := range c.getKeysForParameter(key) {
		value := c[k]
		var valErrs []error

		isRequired := false
		for _, v := range param.Validations {
			if _, ok := v.(ValidationRequired); ok {
				isRequired = true
			}
			err := v.Validate(value)
			if err != nil {
				valErrs = append(valErrs, fmt.Errorf("error validating %q: %w", k, err))
				continue
			}
		}
		if value == "" && !isRequired {
			continue // empty optional parameter is valid
		}
		errs = append(errs, valErrs...)
	}

	return errors.Join(errs...)
}

func (c Config) getKeysForParameter(key string) []string {
	// First break up the key into tokens.
	tokens := strings.Split(key, "*")
	if len(tokens) == 1 {
		// No wildcard in the key, return the key directly.
		return []string{key}
	}

	// There is at least one wildcard in the key, we need to manually find all
	// the keys that match the pattern.
	var keys []string
	for k := range c {
		fullKey := k
		for i, token := range tokens {
			if i == len(tokens)-1 {
				if k == "" && token != "" {
					// The key is consumed, but the token is not, it does not match the pattern.
					// This happens when the last token is not a wildcard and
					// the key is a leaf.
					// e.g. param: "collection.*.format", key: "collection.foo"
					break
				}
				// The last token doesn't matter, if the key matched so far, all
				// wildcards have matched and we can potentially expect a match.
				// The reason for this is so that we can apply defaults to the
				// wildcard keys, even if they don't contain a value in the
				// configuration.
				if token != "" {
					// Build potential key
					fullKey = strings.TrimSuffix(fullKey, k)
					fullKey += token
				}
				keys = append(keys, fullKey)
				break
			}

			var ok bool
			k, ok = consume(k, token)
			if !ok {
				// The key does not start with the token, it does not match the pattern.
				break
			}

			// Between tokens there is a wildcard, we need to consume the key
			// until the next ".". If there is no next ".", the whole key is
			// consumed.
			// e.g. "foo.format" -> ".format" or "foo" -> ""
			if index := strings.IndexRune(k, '.'); index != -1 {
				k = k[index:]
			} else {
				k = ""
			}
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

func (c Config) matchParameterKey(key, pattern string) bool {
	tokens := strings.Split(pattern, "*")
	if len(tokens) == 1 {
		// No wildcard in the key, compare the key directly.
		return key == pattern
	}
	k := key
	for _, token := range tokens {
		var ok bool
		k, ok = consume(k, token)
		if !ok {
			return false
		}

		// Between tokens there is a wildcard, we need to strip the key until
		// the next ".".
		_, k, ok = strings.Cut(k, ".")
		if ok {
			k = "." + k // Add the "." back to the key.
		}
	}
	return true
}

// DecodeInto copies configuration values into the target object.
// Under the hood, this function uses github.com/mitchellh/mapstructure, with
// the "mapstructure" tag renamed to "json". To rename a key, use the "json"
// tag. To embed structs, append ",squash" to your tag. For more details and
// docs, see https://pkg.go.dev/github.com/mitchellh/mapstructure.
func (c Config) DecodeInto(target any, hookFunc ...mapstructure.DecodeHookFunc) error {
	dConfig := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &target,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			append(
				hookFunc,
				mapStringHookFunc(),
				mapStructHookFunc(),
				emptyStringToZeroValueHookFunc(),
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			)...,
		),
		TagName: "json",
		Squash:  true,
	}

	decoder, err := mapstructure.NewDecoder(dConfig)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}
	err = decoder.Decode(c.breakUp())
	if err != nil {
		return fmt.Errorf("failed to decode configuration map into target: %w", err)
	}

	return nil
}

// breakUp breaks up the configuration into a map of maps based on the dot separator.
func (c Config) breakUp() map[string]any {
	const sep = "."

	brokenUp := make(map[string]any)
	for k, v := range c {
		// break up based on dot and put in maps in case target struct is broken up
		tokens := strings.Split(k, sep)
		remain := k
		current := brokenUp
		for _, t := range tokens {
			current[remain] = v // we don't care if we overwrite a map here, the string has precedence
			if _, ok := current[t]; !ok {
				current[t] = map[string]any{}
			}
			var ok bool
			current, ok = current[t].(map[string]any)
			if !ok {
				break // this key is a string, leave it as it is
			}
			_, remain, _ = strings.Cut(remain, sep)
		}
	}
	return brokenUp
}

func emptyStringToZeroValueHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String || data != "" {
			return data, nil
		}
		return reflect.New(t).Elem().Interface(), nil
	}
}

func mapStringHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		if f.Kind() != reflect.Map || f.Elem().Kind() != reflect.Interface ||
			t.Kind() != reflect.Map || t.Elem().Kind() != reflect.String {
			return data, nil
		}

		//nolint:forcetypeassert // We checked in the condition above and know it's a map[string]any
		dataMap := data.(map[string]any)

		// remove all keys with maps
		for k, v := range dataMap {
			if reflect.TypeOf(v).Kind() == reflect.Map {
				delete(dataMap, k)
			}
		}

		return dataMap, nil
	}
}

func mapStructHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		if f.Kind() != reflect.Map || f.Elem().Kind() != reflect.Interface ||
			t.Kind() != reflect.Map || t.Elem().Kind() != reflect.Struct {
			return data, nil
		}

		//nolint:forcetypeassert // We checked in the condition above and know it's a map[string]any
		dataMap := data.(map[string]any)

		// remove all keys with a dot that contains a value with a string
		for k, v := range dataMap {
			_, isString := v.(string)
			if !isString || !strings.Contains(k, ".") {
				continue
			}
			delete(dataMap, k)
		}

		return dataMap, nil
	}
}

func consume(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		// The key does not start with the token, it does not match the pattern.
		return "", false
	}
	return strings.TrimPrefix(s, prefix), true
}
