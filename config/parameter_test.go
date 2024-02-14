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

// func TestParameters_ToProto(t *testing.T) {
// 	is := is.New(t)
// 	validations := []Validation{
// 		ValidationRequired{},
// 		ValidationLessThan{5.10},
// 		ValidationGreaterThan{0},
// 		ValidationInclusion{[]string{"1", "2"}},
// 		ValidationExclusion{[]string{"3", "4"}},
// 		ValidationRegex{regexp.MustCompile("[a-z]*")},
// 	}
// 	want := []parameterv1.Validation{
// 		{
// 			Type:  parameterv1.Validation_TYPE_REQUIRED,
// 			Value: "",
// 		}, {
// 			Type:  parameterv1.Validation_TYPE_LESS_THAN,
// 			Value: "5.1",
// 		}, {
// 			Type:  parameterv1.Validation_TYPE_GREATER_THAN,
// 			Value: "0",
// 		}, {
// 			Type:  parameterv1.Validation_TYPE_INCLUSION,
// 			Value: "1,2",
// 		}, {
// 			Type:  parameterv1.Validation_TYPE_EXCLUSION,
// 			Value: "3,4",
// 		}, {
// 			Type:  parameterv1.Validation_TYPE_REGEX,
// 			Value: "[a-z]*",
// 		},
// 	}
// 	got := convertValidations(validations)
// 	is.Equal(got, want)
// }
