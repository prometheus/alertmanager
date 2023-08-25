// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	matchers "github.com/prometheus/alertmanager/matchers/parse"
)

var justLex bool

func printMatchers(s string) {
	m, err := matchers.Parse(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Fprintln(os.Stdout, m)
	}
}

func printTokens(s string) {
	l := matchers.NewLexer(strings.TrimSpace(s))
	for {
		tok, err := l.Scan()
		if err != nil {
			fmt.Fprintln(os.Stdout, err)
			break
		} else if !tok.IsEOF() {
			fmt.Fprintln(os.Stdout, tok)
		} else {
			break
		}
	}
}

func switchMode() {
	justLex = !justLex
	if justLex {
		fmt.Fprintln(os.Stderr, "Switched to lex mode")
	} else {
		fmt.Fprintln(os.Stdout, "Switched to parse mode")
	}
}

func main() {
	fmt.Fprintln(os.Stdout, "Welcome to the repl! Use w to switch between lex and parse modes.")
	r := bufio.NewReader(os.Stdin)
	for {
		in, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			fmt.Fprintf(os.Stderr, "unexpected error: %s\n", err)
		}
		if strings.TrimSpace(in) == "w" {
			switchMode()
			continue
		}
		if justLex {
			printTokens(in)
		} else {
			printMatchers(in)
		}
	}
}
