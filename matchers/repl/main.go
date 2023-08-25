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
