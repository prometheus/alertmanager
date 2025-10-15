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
		expected []token
		err      string
	}{{
		name:  "no input",
		input: "",
	}, {
		name:  "open brace",
		input: "{",
		expected: []token{{
			kind:  tokenOpenBrace,
			value: "{",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
	}, {
		name:  "open brace with leading space",
		input: " {",
		expected: []token{{
			kind:  tokenOpenBrace,
			value: "{",
			position: position{
				offsetStart: 1,
				offsetEnd:   2,
				columnStart: 1,
				columnEnd:   2,
			},
		}},
	}, {
		name:  "close brace",
		input: "}",
		expected: []token{{
			kind:  tokenCloseBrace,
			value: "}",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
	}, {
		name:  "close brace with leading space",
		input: " }",
		expected: []token{{
			kind:  tokenCloseBrace,
			value: "}",
			position: position{
				offsetStart: 1,
				offsetEnd:   2,
				columnStart: 1,
				columnEnd:   2,
			},
		}},
	}, {
		name:  "open and closing braces",
		input: "{}",
		expected: []token{{
			kind:  tokenOpenBrace,
			value: "{",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}, {
			kind:  tokenCloseBrace,
			value: "}",
			position: position{
				offsetStart: 1,
				offsetEnd:   2,
				columnStart: 1,
				columnEnd:   2,
			},
		}},
	}, {
		name:  "open and closing braces with space",
		input: "{ }",
		expected: []token{{
			kind:  tokenOpenBrace,
			value: "{",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}, {
			kind:  tokenCloseBrace,
			value: "}",
			position: position{
				offsetStart: 2,
				offsetEnd:   3,
				columnStart: 2,
				columnEnd:   3,
			},
		}},
	}, {
		name:  "unquoted",
		input: "hello",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
	}, {
		name:  "unquoted with underscore",
		input: "hello_world",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello_world",
			position: position{
				offsetStart: 0,
				offsetEnd:   11,
				columnStart: 0,
				columnEnd:   11,
			},
		}},
	}, {
		name:  "unquoted with colon",
		input: "hello:world",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello:world",
			position: position{
				offsetStart: 0,
				offsetEnd:   11,
				columnStart: 0,
				columnEnd:   11,
			},
		}},
	}, {
		name:  "unquoted with numbers",
		input: "hello0123456789",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello0123456789",
			position: position{
				offsetStart: 0,
				offsetEnd:   15,
				columnStart: 0,
				columnEnd:   15,
			},
		}},
	}, {
		name:  "unquoted can start with underscore",
		input: "_hello",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "_hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   6,
				columnStart: 0,
				columnEnd:   6,
			},
		}},
	}, {
		name:  "unquoted separated with space",
		input: "hello world",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}, {
			kind:  tokenUnquoted,
			value: "world",
			position: position{
				offsetStart: 6,
				offsetEnd:   11,
				columnStart: 6,
				columnEnd:   11,
			},
		}},
	}, {
		name:  "newline before unquoted is skipped",
		input: "\nhello",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 1,
				offsetEnd:   6,
				columnStart: 1,
				columnEnd:   6,
			},
		}},
	}, {
		name:  "newline after unquoted is skipped",
		input: "hello\n",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
	}, {
		name:  "carriage return before unquoted is skipped",
		input: "\rhello",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 1,
				offsetEnd:   6,
				columnStart: 1,
				columnEnd:   6,
			},
		}},
	}, {
		name:  "space before unquoted is skipped",
		input: " hello",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 1,
				offsetEnd:   6,
				columnStart: 1,
				columnEnd:   6,
			},
		}},
	}, {
		name:  "space after unquoted is skipped",
		input: "hello ",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
	}, {
		name:  "newline between two unquoted is skipped",
		input: "hello\nworld",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}, {
			kind:  tokenUnquoted,
			value: "world",
			position: position{
				offsetStart: 6,
				offsetEnd:   11,
				columnStart: 6,
				columnEnd:   11,
			},
		}},
	}, {
		name:  "unquoted $",
		input: "$",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "$",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
	}, {
		name:  "unquoted emoji",
		input: "🙂",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "🙂",
			position: position{
				offsetStart: 0,
				offsetEnd:   4,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
	}, {
		name:  "unquoted unicode",
		input: "Σ",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "Σ",
			position: position{
				offsetStart: 0,
				offsetEnd:   2,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
	}, {
		name:  "unquoted unicode sentence",
		input: "hello🙂Σ world",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello🙂Σ",
			position: position{
				offsetStart: 0,
				offsetEnd:   11,
				columnStart: 0,
				columnEnd:   7,
			},
		}, {
			kind:  tokenUnquoted,
			value: "world",
			position: position{
				offsetStart: 12,
				offsetEnd:   17,
				columnStart: 8,
				columnEnd:   13,
			},
		}},
	}, {
		name:  "unquoted unicode sentence with unicode space",
		input: "hello🙂Σ\u202fworld",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello🙂Σ",
			position: position{
				offsetStart: 0,
				offsetEnd:   11,
				columnStart: 0,
				columnEnd:   7,
			},
		}, {
			kind:  tokenUnquoted,
			value: "world",
			position: position{
				offsetStart: 14,
				offsetEnd:   19,
				columnStart: 8,
				columnEnd:   13,
			},
		}},
	}, {
		name:  "quoted",
		input: "\"hello\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   7,
				columnStart: 0,
				columnEnd:   7,
			},
		}},
	}, {
		name:  "quoted with unicode",
		input: "\"hello 🙂\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello 🙂\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   12,
				columnStart: 0,
				columnEnd:   9,
			},
		}},
	}, {
		name:  "quoted with space",
		input: "\"hello world\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello world\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   13,
				columnStart: 0,
				columnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with unicode space",
		input: "\"hello\u202fworld\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello\u202fworld\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   15,
				columnStart: 0,
				columnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with newline",
		input: "\"hello\nworld\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello\nworld\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   13,
				columnStart: 0,
				columnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with tab",
		input: "\"hello\tworld\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello\tworld\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   13,
				columnStart: 0,
				columnEnd:   13,
			},
		}},
	}, {
		name:  "quoted with regex digit character class",
		input: "\"\\d+\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"\\d+\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
	}, {
		name:  "quoted with escaped regex digit character class",
		input: "\"\\\\d+\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"\\\\d+\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   6,
				columnStart: 0,
				columnEnd:   6,
			},
		}},
	}, {
		name:  "quoted with escaped quotes",
		input: "\"hello \\\"world\\\"\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello \\\"world\\\"\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   17,
				columnStart: 0,
				columnEnd:   17,
			},
		}},
	}, {
		name:  "quoted with escaped backslash",
		input: "\"hello world\\\\\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"hello world\\\\\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   15,
				columnStart: 0,
				columnEnd:   15,
			},
		}},
	}, {
		name:  "quoted escape sequence",
		input: "\"\\n\"",
		expected: []token{{
			kind:  tokenQuoted,
			value: "\"\\n\"",
			position: position{
				offsetStart: 0,
				offsetEnd:   4,
				columnStart: 0,
				columnEnd:   4,
			},
		}},
	}, {
		name:  "equals operator",
		input: "=",
		expected: []token{{
			kind:  tokenEquals,
			value: "=",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
	}, {
		name:  "not equals operator",
		input: "!=",
		expected: []token{{
			kind:  tokenNotEquals,
			value: "!=",
			position: position{
				offsetStart: 0,
				offsetEnd:   2,
				columnStart: 0,
				columnEnd:   2,
			},
		}},
	}, {
		name:  "matches regex operator",
		input: "=~",
		expected: []token{{
			kind:  tokenMatches,
			value: "=~",
			position: position{
				offsetStart: 0,
				offsetEnd:   2,
				columnStart: 0,
				columnEnd:   2,
			},
		}},
	}, {
		name:  "not matches regex operator",
		input: "!~",
		expected: []token{{
			kind:  tokenNotMatches,
			value: "!~",
			position: position{
				offsetStart: 0,
				offsetEnd:   2,
				columnStart: 0,
				columnEnd:   2,
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
		name:  "unexpected ! after operator",
		input: "=!",
		expected: []token{{
			kind:  tokenEquals,
			value: "=",
			position: position{
				offsetStart: 0,
				offsetEnd:   1,
				columnStart: 0,
				columnEnd:   1,
			},
		}},
		err: "1:2: unexpected end of input, expected one of '=~'",
	}, {
		name:  "unexpected !! after operator",
		input: "!=!!",
		expected: []token{{
			kind:  tokenNotEquals,
			value: "!=",
			position: position{
				offsetStart: 0,
				offsetEnd:   2,
				columnStart: 0,
				columnEnd:   2,
			},
		}},
		err: "2:3: !: expected one of '=~'",
	}, {
		name:  "unexpected ! after unquoted",
		input: "hello!",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
		err: "5:6: unexpected end of input, expected one of '=~'",
	}, {
		name:  "invalid escape sequence",
		input: "\\n",
		err:   "0:1: \\: invalid input",
	}, {
		name:  "invalid escape sequence before unquoted",
		input: "\\nhello",
		err:   "0:1: \\: invalid input",
	}, {
		name:  "invalid escape sequence after unquoted",
		input: "hello\\n",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
		err: "5:6: \\: invalid input",
	}, {
		name:  "another invalid escape sequence after unquoted",
		input: "hello\\r",
		expected: []token{{
			kind:  tokenUnquoted,
			value: "hello",
			position: position{
				offsetStart: 0,
				offsetEnd:   5,
				columnStart: 0,
				columnEnd:   5,
			},
		}},
		err: "5:6: \\: invalid input",
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
			l := lexer{input: test.input}
			// scan all expected tokens.
			for i := 0; i < len(test.expected); i++ {
				tok, err := l.scan()
				require.NoError(t, err)
				require.Equal(t, test.expected[i], tok)
			}
			if test.err == "" {
				// Check there are no more tokens.
				tok, err := l.scan()
				require.NoError(t, err)
				require.Equal(t, token{}, tok)
			} else {
				// Check if expected error is returned.
				tok, err := l.scan()
				require.Equal(t, token{}, tok)
				require.EqualError(t, err, test.err)
			}
		})
	}
}

// This test asserts that the lexer does not emit more tokens after an
// error has occurred.
func TestLexer_ScanError(t *testing.T) {
	l := lexer{input: "\"hello"}
	for i := 0; i < 10; i++ {
		tok, err := l.scan()
		require.Equal(t, token{}, tok)
		require.EqualError(t, err, "0:6: \"hello: missing end \"")
	}
}

func TestLexer_Peek(t *testing.T) {
	l := lexer{input: "hello world"}
	expected1 := token{
		kind:  tokenUnquoted,
		value: "hello",
		position: position{
			offsetStart: 0,
			offsetEnd:   5,
			columnStart: 0,
			columnEnd:   5,
		},
	}
	expected2 := token{
		kind:  tokenUnquoted,
		value: "world",
		position: position{
			offsetStart: 6,
			offsetEnd:   11,
			columnStart: 6,
			columnEnd:   11,
		},
	}
	// Check that peek() returns the first token.
	tok, err := l.peek()
	require.NoError(t, err)
	require.Equal(t, expected1, tok)
	// Check that scan() returns the peeked token.
	tok, err = l.scan()
	require.NoError(t, err)
	require.Equal(t, expected1, tok)
	// Check that peek() returns the second token until the next scan().
	for i := 0; i < 10; i++ {
		tok, err = l.peek()
		require.NoError(t, err)
		require.Equal(t, expected2, tok)
	}
	// Check that scan() returns the last token.
	tok, err = l.scan()
	require.NoError(t, err)
	require.Equal(t, expected2, tok)
	// Should not be able to peek() further tokens.
	for i := 0; i < 10; i++ {
		tok, err = l.peek()
		require.NoError(t, err)
		require.Equal(t, token{}, tok)
	}
}

// This test asserts that the lexer does not emit more tokens after an
// error has occurred.
func TestLexer_PeekError(t *testing.T) {
	l := lexer{input: "\"hello"}
	for i := 0; i < 10; i++ {
		tok, err := l.peek()
		require.Equal(t, token{}, tok)
		require.EqualError(t, err, "0:6: \"hello: missing end \"")
	}
}

func TestLexer_Pos(t *testing.T) {
	l := lexer{input: "hello🙂"}
	// The start position should be the zero-value.
	require.Equal(t, position{}, l.position())
	_, err := l.scan()
	require.NoError(t, err)
	// The position should contain the offset and column of the end.
	expected := position{
		offsetStart: 9,
		offsetEnd:   9,
		columnStart: 6,
		columnEnd:   6,
	}
	require.Equal(t, expected, l.position())
	// The position should not change once the input has been consumed.
	tok, err := l.scan()
	require.NoError(t, err)
	require.True(t, tok.isEOF())
	require.Equal(t, expected, l.position())
}
