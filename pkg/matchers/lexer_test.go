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

package matchers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexer_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Token
		err      string
	}{{
		name:  "open paren",
		input: "{",
		expected: []Token{
			{Kind: TokenOpenParen, Value: "{", Start: 0, End: 1},
		},
	}, {
		name:  "open paren with space",
		input: " {",
		expected: []Token{
			{Kind: TokenOpenParen, Value: "{", Start: 1, End: 2},
		},
	}, {
		name:  "close paren",
		input: "}",
		expected: []Token{
			{Kind: TokenCloseParen, Value: "}", Start: 0, End: 1},
		},
	}, {
		name:  "close paren with space",
		input: "}",
		expected: []Token{
			{Kind: TokenCloseParen, Value: "}", Start: 0, End: 1},
		},
	}, {
		name:  "open and closing parens",
		input: "{}",
		expected: []Token{
			{Kind: TokenOpenParen, Value: "{", Start: 0, End: 1},
			{Kind: TokenCloseParen, Value: "}", Start: 1, End: 2},
		},
	}, {
		name:  "open and closing parens with space",
		input: "{ }",
		expected: []Token{
			{Kind: TokenOpenParen, Value: "{", Start: 0, End: 1},
			{Kind: TokenCloseParen, Value: "}", Start: 2, End: 3},
		},
	}, {
		name:  "ident",
		input: "hello",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello", Start: 0, End: 5},
		},
	}, {
		name:  "ident with underscore",
		input: "hello_world",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello_world", Start: 0, End: 11},
		},
	}, {
		name:  "ident with colon",
		input: "hello:world",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello:world", Start: 0, End: 11},
		},
	}, {
		name:  "ident with numbers",
		input: "hello0123456789",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello0123456789", Start: 0, End: 15},
		},
	}, {
		name:  "ident can start with underscore",
		input: "_hello",
		expected: []Token{
			{Kind: TokenIdent, Value: "_hello", Start: 0, End: 6},
		},
	}, {
		name:  "idents separated with space",
		input: "hello world",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello", Start: 0, End: 5},
			{Kind: TokenIdent, Value: "world", Start: 6, End: 11},
		},
	}, {
		name:  "quoted",
		input: "\"hello\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello\"", Start: 0, End: 7},
		},
	}, {
		name:  "quoted with unicode",
		input: "\"hello 🙂\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello 🙂\"", Start: 0, End: 12},
		},
	}, {
		name:  "quoted with space",
		input: "\"hello world\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello world\"", Start: 0, End: 13},
		},
	}, {
		name:  "quoted with tab",
		input: "\"hello\tworld\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello\tworld\"", Start: 0, End: 13},
		},
	}, {
		name:  "quoted with newline",
		input: "\"hello\nworld\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello\nworld\"", Start: 0, End: 13},
		},
	}, {
		name:  "quoted with escaped quotes",
		input: "\"hello \\\"world\\\"\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello \\\"world\\\"\"", Start: 0, End: 17},
		},
	}, {
		name:  "quoted with escaped backticks",
		input: "\"hello \\world\"",
		expected: []Token{
			{Kind: TokenQuoted, Value: "\"hello \\world\"", Start: 0, End: 14},
		},
	}, {
		name:  "equals operator",
		input: "=",
		expected: []Token{
			{Kind: TokenOperator, Value: "=", Start: 0, End: 1},
		},
	}, {
		name:  "not equals operator",
		input: "!=",
		expected: []Token{
			{Kind: TokenOperator, Value: "!=", Start: 0, End: 2},
		},
	}, {
		name:  "matches regex operator",
		input: "=~",
		expected: []Token{
			{Kind: TokenOperator, Value: "=~", Start: 0, End: 2},
		},
	}, {
		name:  "not matches regex operator",
		input: "!~",
		expected: []Token{
			{Kind: TokenOperator, Value: "!~", Start: 0, End: 2},
		},
	}, {
		name:  "unexpected $",
		input: "$",
		err:   "0:1: $: invalid input",
	}, {
		name:  "unexpected emoji",
		input: "🙂",
		err:   "0:4: 🙂: invalid input",
	}, {
		name:  "unexpected unicode letter",
		input: "Σ",
		err:   "0:2: Σ: invalid input",
	}, {
		name:  "unexpected : at start of ident",
		input: ":hello",
		err:   "0:1: :: invalid input",
	}, {
		name:  "unexpected $ in ident",
		input: "hello$",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello", Start: 0, End: 5},
		},
		err: "5:6: $: invalid input",
	}, {
		name:  "unexpected unicode letter in ident",
		input: "helloΣ",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello", Start: 0, End: 5},
		},
		err: "5:7: Σ: invalid input",
	}, {
		name:  "unexpected emoji in ident",
		input: "hello🙂",
		expected: []Token{
			{Kind: TokenIdent, Value: "hello", Start: 0, End: 5},
		},
		err: "5:9: 🙂: invalid input",
	}, {
		name:  "invalid operator",
		input: "!",
		err:   "0:1: expected one of '=~', got EOF",
	}, {
		name:  "another invalid operator",
		input: "~",
		err:   "0:1: ~: invalid input",
	}, {
		name:  "unexpected $ in operator",
		input: "=$",
		expected: []Token{
			{Kind: TokenOperator, Value: "=", Start: 0, End: 1},
		},
		err: "1:2: $: invalid input",
	}, {
		name:  "unexpected ! after operator",
		input: "=!",
		expected: []Token{
			{Kind: TokenOperator, Value: "=", Start: 0, End: 1},
		},
		err: "1:2: expected one of '=~', got EOF",
	}, {
		name:  "unexpected !! after operator",
		input: "!=!!",
		expected: []Token{
			{Kind: TokenOperator, Value: "!=", Start: 0, End: 2},
		},
		err: "2:3: expected one of '=~', got '!'",
	}, {
		name:  "unterminated quoted",
		input: "\"hello",
		err:   "0:6: expected one of '\"', got EOF",
	}, {
		name:  "unterminated quoted with escaped quote",
		input: "\"hello\\\"",
		err:   "0:8: expected one of '\"', got EOF",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			l := NewLexer(test.input)
			// scan all expected tokens
			for i := 0; i < len(test.expected); i++ {
				tok, err := l.Scan()
				require.NoError(t, err)
				assert.Equal(t, test.expected[i], tok)
			}
			if test.err == "" {
				// check there are no more tokens
				tok, err := l.Scan()
				require.NoError(t, err)
				assert.Equal(t, Token{}, tok)
			} else {
				// check if expected error is returned
				tok, err := l.Scan()
				assert.Equal(t, Token{}, tok)
				assert.EqualError(t, err, test.err)
			}
		})
	}
}

// This test asserts that the lexer does not emit more tokens after an
// error has occurred.
func TestLexer_ScanError(t *testing.T) {
	l := NewLexer("\"hello")
	for i := 0; i < 10; i++ {
		tok, err := l.Scan()
		assert.Equal(t, Token{}, tok)
		assert.EqualError(t, err, "0:6: expected one of '\"', got EOF")
	}
}

func TestLexer_Peek(t *testing.T) {
	l := NewLexer("hello world")
	expected1 := Token{Kind: TokenIdent, Value: "hello", Start: 0, End: 5}
	expected2 := Token{Kind: TokenIdent, Value: "world", Start: 6, End: 11}
	// check that Peek() returns the first token
	tok, err := l.Peek()
	assert.NoError(t, err)
	assert.Equal(t, expected1, tok)
	// check that Scan() returns the peeked token
	tok, err = l.Scan()
	assert.NoError(t, err)
	assert.Equal(t, expected1, tok)
	// check that Peek() returns the second token until the next Scan()
	for i := 0; i < 10; i++ {
		tok, err = l.Peek()
		assert.NoError(t, err)
		assert.Equal(t, expected2, tok)
	}
	// check that Scan() returns the last token
	tok, err = l.Scan()
	assert.NoError(t, err)
	assert.Equal(t, expected2, tok)
	// should not be able to Peek() further tokens
	for i := 0; i < 10; i++ {
		tok, err = l.Peek()
		assert.NoError(t, err)
		assert.Equal(t, Token{}, tok)
	}
}

// This test asserts that the lexer does not emit more tokens after an
// error has occurred.
func TestLexer_PeekError(t *testing.T) {
	l := NewLexer("\"hello")
	for i := 0; i < 10; i++ {
		tok, err := l.Peek()
		assert.Equal(t, Token{}, tok)
		assert.EqualError(t, err, "0:6: expected one of '\"', got EOF")
	}
}
