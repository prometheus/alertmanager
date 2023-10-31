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
