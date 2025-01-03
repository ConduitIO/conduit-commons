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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/conduitio/conduit-commons/paramgen/paramgen"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("paramgen: ")

	// parse the command arguments
	args := parseFlags()

	// parse the sdk parameters
	params, pkg, err := paramgen.ParseParameters(args.path, args.structName)
	if err != nil {
		log.Fatalf("error: failed to parse parameters: %v", err)
	}

	code := paramgen.GenerateCode(params, pkg, args.structName)

	path := strings.TrimSuffix(args.path, "/") + "/" + args.output
	err = os.WriteFile(path, []byte(code), 0o600)
	if err != nil {
		log.Fatalf("error: failed to output file: %v", err)
	}
}

type Args struct {
	output     string
	path       string
	structName string
}

func parseFlags() Args {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	var (
		output = flags.String("output", "paramgen.go", "name of the output file")
		path   = flags.String("path", ".", "directory path to the package that has the configuration struct")
	)

	// flags is set up to exit on error, we can safely ignore the error
	_ = flags.Parse(os.Args[1:])

	if len(flags.Args()) == 0 {
		log.Println("error: struct name argument missing")
		fmt.Println()
		flags.Usage()
		os.Exit(1)
	}

	var args Args
	args.output = stringPtrToVal(output)
	args.path = stringPtrToVal(path)
	args.structName = flags.Args()[0]

	// add .go suffix if it is not in the name
	if !strings.HasSuffix(args.output, ".go") {
		args.output += ".go"
	}

	return args
}

func stringPtrToVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
