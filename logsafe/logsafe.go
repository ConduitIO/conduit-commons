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

// Package logsafe holds the one helper the Conduit ecosystem uses to refer to
// record data in a log line or an error without writing the data.
//
// Record keys, positions (which embed key values for some connectors) and
// payload values must not appear in logs or in errors: errors reach the DLQ,
// the API and CLI output. A salted digest lets two lines about the same value
// in one process be matched, which is all a log reader needs.
package logsafe

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// salt is random per process: a logged digest cannot be reversed by hashing
// guessed values (record keys are often sequential ids) and cannot be matched
// across processes or restarts.
var salt = func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("logsafe: cannot read random bytes for the salt: %v", err))
	}
	return b
}()

// Hash returns a short (16 hex characters), salted digest of b. The same bytes
// give the same digest within a process. Never log the bytes themselves; log
// their length and this.
func Hash(b []byte) string {
	mac := hmac.New(sha256.New, salt)
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

// HashString is Hash for a string.
func HashString(s string) string { return Hash([]byte(s)) }

// HashValue is Hash for a structured value such as a key map: the digest is
// the same for equal values (maps print with sorted keys) and differs between
// types, so 1 and "1" are different keys.
func HashValue(v any) string { return HashString(fmt.Sprintf("%T:%#v", v, v)) }
