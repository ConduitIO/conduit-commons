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

import "errors"

var (
	ErrUnrecognizedParameter    = errors.New("unrecognized parameter")
	ErrInvalidParameterValue    = errors.New("invalid parameter value")
	ErrInvalidParameterType     = errors.New("invalid parameter type")
	ErrInvalidValidationType    = errors.New("invalid validation type")
	ErrRequiredParameterMissing = errors.New("required parameter is not provided")

	ErrLessThanValidationFail    = errors.New("less-than validation failed")
	ErrGreaterThanValidationFail = errors.New("greater-than validation failed")
	ErrInclusionValidationFail   = errors.New("inclusion validation failed")
	ErrExclusionValidationFail   = errors.New("exclusion validation failed")
	ErrRegexValidationFail       = errors.New("regex validation failed")
)
