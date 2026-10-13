// Command hambagen prints the wire-compatibility corpus encoded by
// conduit-commons v0.6.0 (hamba/avro). See gen.sh.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/conduitio/conduit-commons/schema/avro"
)

type fixture struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
	Hex    string `json:"hex"`
}

func main() {
	var out []fixture
	for _, c := range compatCases() {
		serde, err := avro.Parse([]byte(c.schema))
		if err != nil {
			fail(c.name, err)
		}
		b, err := serde.Marshal(c.value())
		if err != nil {
			fail(c.name, err)
		}
		out = append(out, fixture{Name: c.name, Schema: c.schema, Hex: hex.EncodeToString(b)})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fail("encode", err)
	}
}

func fail(what string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
	os.Exit(1)
}
