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

package schema

import "errors"

var (
	// ErrInvalidProtoIsNil is returned when trying to convert a schema object to a
	// proto schema, and the proto is nil.
	ErrInvalidProtoIsNil = errors.New("invalid proto: nil")

	// ErrUnsupportedType is returned when an unsupported type is encountered.
	ErrUnsupportedType = errors.New("unsupported type")

	// ErrResolverRequired is returned by Schema.Serde and
	// Schema.SerdeWithResolver when the schema has References and no
	// Resolver was given to fetch them. Use SerdeWithResolver.
	ErrResolverRequired = errors.New("schema has references but no resolver was given")
)
