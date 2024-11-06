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

package lang

// Ptr returns a pointer to the value passed in.
func Ptr[T any](t T) *T {
	return &t
}

// ValOrZero returns the value of the pointer passed in or the zero value of the
// type if the pointer is nil.
func ValOrZero[T any](t *T) T {
	if t == nil {
		return Zero[T]()
	}
	return *t
}

// Zero returns the zero value of the type passed in.
func Zero[T any]() T {
	var t T
	return t
}
