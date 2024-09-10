// Copyright © 2023 Meroxa, Inc.
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

// Package rabin provides a Rabin fingerprint hash.Hash64 implementation compatible
// with the Avro spec: https://avro.apache.org/docs/1.8.2/spec.html#schema_fingerprints.
package rabin

import "hash"

type digest uint64

// New constructs a new Rabin fingerprint hash.Hash64 initialized to the empty
// state according to the Avro spec: https://avro.apache.org/docs/1.8.2/spec.html#schema_fingerprints.
func New() hash.Hash64 {
	var d digest
	d.Reset()
	return &d
}

func (d *digest) Write(p []byte) (n int, err error) {
	*d = update(*d, p)
	return len(p), nil
}

func (d *digest) Sum64() uint64 {
	return uint64(*d)
}

func (d *digest) Sum(in []byte) []byte {
	s := d.Sum64()
	return append(in, byte(s>>56), byte(s>>48), byte(s>>40), byte(s>>32), byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
}

func (d *digest) Reset() {
	*d = digest(rabinEmpty)
}

func (d *digest) Size() int      { return 8 }
func (d *digest) BlockSize() int { return 1 }

const rabinEmpty = uint64(0xc15d213aa4d7a795)

// rabinTable is used to compute the CRC-64-AVRO fingerprint.
var rabinTable = newRabinFingerprintTable()

// newRabinFingerprintTable initializes the fingerprint table according to the
// spec: https://avro.apache.org/docs/1.8.2/spec.html#schema_fingerprints
func newRabinFingerprintTable() [256]uint64 {
	fpTable := [256]uint64{}
	for i := 0; i < 256; i++ {
		fp := uint64(i) //nolint:gosec // this won't overflow, it's between 0 and 256
		for j := 0; j < 8; j++ {
			fp = (fp >> 1) ^ (rabinEmpty & -(fp & 1))
		}
		fpTable[i] = fp
	}
	return fpTable
}

// Bytes creates a Rabin fingerprint according to the spec:
// https://avro.apache.org/docs/1.8.2/spec.html#schema_fingerprints
func Bytes(buf []byte) uint64 {
	h := New()
	_, _ = h.Write(buf) // it never returns an error
	return h.Sum64()
}

// update adds p to the running checksum d.
func update(d digest, p []byte) digest {
	fp := uint64(d)
	for i := 0; i < len(p); i++ {
		fp = (fp >> 8) ^ rabinTable[(byte(fp)^p[i])&0xff]
	}
	return digest(fp)
}
