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

package parse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLexer_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Token
		err      string
	}{{
		name:  "no input",
		input: "",
	}, {
		name:  "open brace",
		input: "{",
		expected: []Token{{
			Kind:  TokenOpenBrace,
			Value: "{",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
	}, {
		name:  "open brace with leading space",
		input: " {",
		expected: []Token{{
			Kind:  TokenOpenBrace,
			Value: "{",
			Position: Position{
				OffsetStart: 1,
				OffsetEnd:   2,
				ColumnStart: 1,
				ColumnEnd:   2,
			},
		}},
	}, {
		name:  "close brace",
		input: "}",
		expected: []Token{{
			Kind:  TokenCloseBrace,
			Value: "}",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
	}, {
		name:  "close brace with leading space",
		input: " }",
		expected: []Token{{
			Kind:  TokenCloseBrace,
			Value: "}",
			Position: Position{
				OffsetStart: 1,
				OffsetEnd:   2,
				ColumnStart: 1,
				ColumnEnd:   2,
			},
		}},
	}, {
		name:  "open and closing braces",
		input: "{}",
		expected: []Token{{
			Kind:  TokenOpenBrace,
			Value: "{",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}, {
			Kind:  TokenCloseBrace,
			Value: "}",
			Position: Position{
				OffsetStart: 1,
				OffsetEnd:   2,
				ColumnStart: 1,
				ColumnEnd:   2,
			},
		}},
	}, {
		name:  "open and closing braces with space",
		input: "{ }",
		expected: []Token{{
			Kind:  TokenOpenBrace,
			Value: "{",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}, {
			Kind:  TokenCloseBrace,
			Value: "}",
			Position: Position{
				OffsetStart: 2,
				OffsetEnd:   3,
				ColumnStart: 2,
				ColumnEnd:   3,
			},
		}},
	}, {
		name:  "unquoted",
		input: "hello",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   5,
				ColumnStart: 0,
				ColumnEnd:   5,
			},
		}},
	}, {
		name:  "unquoted with underscore",
		input: "hello_world",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello_world",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   11,
				ColumnStart: 0,
				ColumnEnd:   11,
			},
		}},
	}, {
		name:  "unquoted with colon",
		input: "hello:world",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello:world",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   11,
				ColumnStart: 0,
				ColumnEnd:   11,
			},
		}},
	}, {
		name:  "unquoted with numbers",
		input: "hello0123456789",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello0123456789",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   15,
				ColumnStart: 0,
				ColumnEnd:   15,
			},
		}},
	}, {
		name:  "unquoted can start with underscore",
		input: "_hello",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "_hello",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   6,
				ColumnStart: 0,
				ColumnEnd:   6,
			},
		}},
	}, {
		name:  "unquoted separated with space",
		input: "hello world",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   5,
				ColumnStart: 0,
				ColumnEnd:   5,
			},
		}, {
			Kind:  TokenUnquoted,
			Value: "world",
			Position: Position{
				OffsetStart: 6,
				OffsetEnd:   11,
				ColumnStart: 6,
				ColumnEnd:   11,
			},
		}},
	}, {
		name:  "unquoted $",
		input: "$",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "$",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
	}, {
		name:  "unquoted emoji",
		input: "🙂",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "🙂",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   4,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
	}, {
		name:  "unquoted unicode",
		input: "Σ",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "Σ",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   2,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
	}, {
		name:  "unquoted unicode sentence",
		input: "hello🙂Σ world",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello🙂Σ",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   11,
				ColumnStart: 0,
				ColumnEnd:   7,
			},
		}, {
			Kind:  TokenUnquoted,
			Value: "world",
			Position: Position{
				OffsetStart: 12,
				OffsetEnd:   17,
				ColumnStart: 8,
				ColumnEnd:   13,
			},
		}},
	}, {
		name:  "unquoted unicode sentence with unicode space",
		input: "hello🙂Σ\u202fworld",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello🙂Σ",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   11,
				ColumnStart: 0,
				ColumnEnd:   7,
			},
		}, {
			Kind:  TokenUnquoted,
			Value: "world",
			Position: Position{
				OffsetStart: 14,
				OffsetEnd:   19,
				ColumnStart: 8,
				ColumnEnd:   13,
			},
		}},
	}, {
		name:  "quoted",
		input: "\"hello\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   7,
				ColumnStart: 0,
				ColumnEnd:   7,
			},
		}},
	}, {
		name:  "quoted with unicode",
		input: "\"hello 🙂\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello 🙂\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   12,
				ColumnStart: 0,
				ColumnEnd:   9,
			},
		}},
	}, {
		name:  "quoted with space",
		input: "\"hello world\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello world\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   13,
				ColumnStart: 0,
				ColumnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with unicode space",
		input: "\"hello\u202fworld\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello\u202fworld\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   15,
				ColumnStart: 0,
				ColumnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with newline",
		input: "\"hello\nworld\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello\nworld\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   13,
				ColumnStart: 0,
				ColumnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with tab",
		input: "\"hello\tworld\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello\tworld\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   13,
				ColumnStart: 0,
				ColumnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with escaped quotes",
		input: "\"hello \\\"world\\\"\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello \\\"world\\\"\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   17,
				ColumnStart: 0,
				ColumnEnd:   17,
			},
		}},
	}, {
		name:  "quoted with escaped backslash",
		input: "\"hello world\\\\\"",
		expected: []Token{{
			Kind:  TokenQuoted,
			Value: "\"hello world\\\\\"",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   15,
				ColumnStart: 0,
				ColumnEnd:   15,
			},
		}},
	}, {
		name:  "equals operator",
		input: "=",
		expected: []Token{{
			Kind:  TokenOperator,
			Value: "=",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
	}, {
		name:  "not equals operator",
		input: "!=",
		expected: []Token{{
			Kind:  TokenOperator,
			Value: "!=",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   2,
				ColumnStart: 0,
				ColumnEnd:   2,
			},
		}},
	}, {
		name:  "matches regex operator",
		input: "=~",
		expected: []Token{{
			Kind:  TokenOperator,
			Value: "=~",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   2,
				ColumnStart: 0,
				ColumnEnd:   2,
			},
		}},
	}, {
		name:  "not matches regex operator",
		input: "!~",
		expected: []Token{{
			Kind:  TokenOperator,
			Value: "!~",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   2,
				ColumnStart: 0,
				ColumnEnd:   2,
			},
		}},
	}, {
		name:  "invalid operator",
		input: "!",
		err:   "0:1: unexpected end of input, expected one of '=~'",
	}, {
		name:  "another invalid operator",
		input: "~",
		err:   "0:1: ~: invalid input",
	}, {
		name:  "unexpected ! after unquoted",
		input: "hello!",
		expected: []Token{{
			Kind:  TokenUnquoted,
			Value: "hello",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   5,
				ColumnStart: 0,
				ColumnEnd:   5,
			},
		}},
		err: "5:6: unexpected end of input, expected one of '=~'",
	}, {
		name:  "unexpected ! after operator",
		input: "=!",
		expected: []Token{{
			Kind:  TokenOperator,
			Value: "=",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   1,
				ColumnStart: 0,
				ColumnEnd:   1,
			},
		}},
		err: "1:2: unexpected end of input, expected one of '=~'",
	}, {
		name:  "unexpected !! after operator",
		input: "!=!!",
		expected: []Token{{
			Kind:  TokenOperator,
			Value: "!=",
			Position: Position{
				OffsetStart: 0,
				OffsetEnd:   2,
				ColumnStart: 0,
				ColumnEnd:   2,
			},
		}},
		err: "2:3: !: expected one of '=~'",
	}, {
		name:  "unterminated quoted",
		input: "\"hello",
		err:   "0:6: \"hello: missing end \"",
	}, {
		name:  "unterminated quoted with escaped quote",
		input: "\"hello\\\"",
		err:   "0:8: \"hello\\\": missing end \"",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			l := NewLexer(test.input)
			// scan all expected tokens
			for i := 0; i < len(test.expected); i++ {
				tok, err := l.Scan()
				require.NoError(t, err)
				require.Equal(t, test.expected[i], tok)
			}
			if test.err == "" {
				// check there are no more tokens
				tok, err := l.Scan()
				require.NoError(t, err)
				require.Equal(t, Token{}, tok)
			} else {
				// check if expected error is returned
				tok, err := l.Scan()
				require.Equal(t, Token{}, tok)
				require.EqualError(t, err, test.err)
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
		require.Equal(t, Token{}, tok)
		require.EqualError(t, err, "0:6: \"hello: missing end \"")
	}
}

func TestLexer_Peek(t *testing.T) {
	l := NewLexer("hello world")
	expected1 := Token{
		Kind:  TokenUnquoted,
		Value: "hello",
		Position: Position{
			OffsetStart: 0,
			OffsetEnd:   5,
			ColumnStart: 0,
			ColumnEnd:   5,
		},
	}
	expected2 := Token{
		Kind:  TokenUnquoted,
		Value: "world",
		Position: Position{
			OffsetStart: 6,
			OffsetEnd:   11,
			ColumnStart: 6,
			ColumnEnd:   11,
		},
	}
	// check that Peek() returns the first token
	tok, err := l.Peek()
	require.NoError(t, err)
	require.Equal(t, expected1, tok)
	// check that Scan() returns the peeked token
	tok, err = l.Scan()
	require.NoError(t, err)
	require.Equal(t, expected1, tok)
	// check that Peek() returns the second token until the next Scan()
	for i := 0; i < 10; i++ {
		tok, err = l.Peek()
		require.NoError(t, err)
		require.Equal(t, expected2, tok)
	}
	// check that Scan() returns the last token
	tok, err = l.Scan()
	require.NoError(t, err)
	require.Equal(t, expected2, tok)
	// should not be able to Peek() further tokens
	for i := 0; i < 10; i++ {
		tok, err = l.Peek()
		require.NoError(t, err)
		require.Equal(t, Token{}, tok)
	}
}

// This test asserts that the lexer does not emit more tokens after an
// error has occurred.
func TestLexer_PeekError(t *testing.T) {
	l := NewLexer("\"hello")
	for i := 0; i < 10; i++ {
		tok, err := l.Peek()
		require.Equal(t, Token{}, tok)
		require.EqualError(t, err, "0:6: \"hello: missing end \"")
	}
}

func TestLexer_Pos(t *testing.T) {
	l := NewLexer("hello🙂")
	// The start position should be the zero-value.
	require.Equal(t, Position{}, l.Pos())
	_, err := l.Scan()
	require.NoError(t, err)
	// The position should contain the offset and column of the end.
	expected := Position{
		OffsetStart: 9,
		OffsetEnd:   9,
		ColumnStart: 6,
		ColumnEnd:   6,
	}
	require.Equal(t, expected, l.Pos())
	// The position should not change once the input has been consumed.
	tok, err := l.Scan()
	require.NoError(t, err)
	require.True(t, tok.IsEOF())
	require.Equal(t, expected, l.Pos())
}
