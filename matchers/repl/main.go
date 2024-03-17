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

	"github.com/prometheus/alertmanager/matchers/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
)

const (
	defaultFmt = "# %7s: %s\n"
)

func printUTF8Matchers(s string) {
	m, err := parse.Matchers(strings.TrimSpace(s))
	if err != nil {
		fmt.Fprintf(os.Stderr, defaultFmt, "utf-8", err)
	} else {
		fmt.Fprintf(os.Stdout, defaultFmt, "utf-8", m)
	}
}

func printClassicMatchers(s string) {
	m, err := labels.ParseMatchers(strings.TrimSpace(s))
	if err != nil {
		fmt.Fprintf(os.Stderr, defaultFmt, "classic", err)
	} else {
		fmt.Fprintf(os.Stdout, defaultFmt, "classic", labels.Matchers(m))
	}
}

func main() {
	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("> ")
		in, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			fmt.Fprintf(os.Stderr, "unexpected error: %s\n", err)
		}
		printUTF8Matchers(in)
		printClassicMatchers(in)
	}
}
