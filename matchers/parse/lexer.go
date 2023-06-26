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
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	eof rune = -1
)

func isAlpha(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isNum(r rune) bool {
	return r >= '0' && r <= '9'
}

// ExpectedError is returned when the next rune does not match what is expected.
type ExpectedError struct {
	input       string
	offsetStart int
	offsetEnd   int
	columnStart int
	columnEnd   int
	expected    string
}

func (e ExpectedError) Error() string {
	if e.offsetEnd >= len(e.input) {
		return fmt.Sprintf("%d:%d: unexpected end of input, expected one of '%s'",
			e.columnStart,
			e.columnEnd,
			e.expected,
		)
	}
	return fmt.Sprintf("%d:%d: %s: expected one of '%s'",
		e.columnStart,
		e.columnEnd,
		e.input[e.offsetStart:e.offsetEnd],
		e.expected,
	)
}

// InvalidInputError is returned when the next rune in the input does not match
// the grammar of Prometheus-like matchers.
type InvalidInputError struct {
	input       string
	offsetStart int
	offsetEnd   int
	columnStart int
	columnEnd   int
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("%d:%d: %s: invalid input",
		e.columnStart,
		e.columnEnd,
		e.input[e.offsetStart:e.offsetEnd],
	)
}

// UnterminatedError is returned when text in quotes does not have a closing quote.
type UnterminatedError struct {
	input       string
	offsetStart int
	offsetEnd   int
	columnStart int
	columnEnd   int
	quote       rune
}

func (e UnterminatedError) Error() string {
	return fmt.Sprintf("%d:%d: %s: missing end %c",
		e.columnStart,
		e.columnEnd,
		e.input[e.offsetStart:e.offsetEnd],
		e.quote,
	)
}

// Lexer scans a sequence of tokens that match the grammar of Prometheus-like
// matchers. A token is emitted for each call to Scan() which returns the
// next token in the input or an error if the input does not conform to the
// grammar. A token can be one of a number of kinds and corresponds to a
// subslice of the input. Once the input has been consumed successive calls to
// Scan() return a TokenNone token.
type Lexer struct {
	input  string
	err    error
	start  int // the offset of the current token
	pos    int // the position of the cursor in the input
	width  int // the width of the last rune
	column int // the column offset of the current token
	cols   int // the number of columns (runes) decoded from the input
}

func NewLexer(input string) Lexer {
	return Lexer{
		input: input,
	}
}

func (l *Lexer) Peek() (Token, error) {
	start := l.start
	pos := l.pos
	width := l.width
	column := l.column
	cols := l.cols
	// Do not reset l.err because we can return it on the next call to Scan()
	defer func() {
		l.start = start
		l.pos = pos
		l.width = width
		l.column = column
		l.cols = cols
	}()
	return l.Scan()
}

func (l *Lexer) Scan() (Token, error) {
	tok := Token{}

	// Do not attempt to emit more tokens if the input is invalid
	if l.err != nil {
		return tok, l.err
	}

	// Iterate over each rune in the input and either emit a token or an error
	for r := l.next(); r != eof; r = l.next() {
		switch {
		case r == '{':
			tok = l.emit(TokenOpenBrace)
			return tok, l.err
		case r == '}':
			tok = l.emit(TokenCloseBrace)
			return tok, l.err
		case r == ',':
			tok = l.emit(TokenComma)
			return tok, l.err
		case r == '=' || r == '!':
			l.rewind()
			tok, l.err = l.scanOperator()
			return tok, l.err
		case r == '"':
			l.rewind()
			tok, l.err = l.scanQuoted()
			return tok, l.err
		case r == '_' || isAlpha(r):
			l.rewind()
			tok, l.err = l.scanIdent()
			return tok, l.err
		case unicode.IsSpace(r):
			l.skip()
		default:
			l.err = InvalidInputError{
				input:       l.input,
				offsetStart: l.start,
				offsetEnd:   l.pos,
				columnStart: l.column,
				columnEnd:   l.cols,
			}
			return tok, l.err
		}
	}

	return tok, l.err
}

func (l *Lexer) scanIdent() (Token, error) {
	for r := l.next(); r != eof; r = l.next() {
		if !isAlpha(r) && !isNum(r) && r != '_' && r != ':' {
			l.rewind()
			break
		}
	}
	return l.emit(TokenIdent), nil
}

func (l *Lexer) scanOperator() (Token, error) {
	if err := l.expect("!="); err != nil {
		return Token{}, err
	}

	// Rewind because we need to know if the rune was an '!' or an '='
	l.rewind()

	// If the first rune is an '!' then it must be followed with either an
	// '=' or '~' to not match a string or regex
	if l.accept("!") {
		if err := l.expect("=~"); err != nil {
			return Token{}, err
		}
		return l.emit(TokenOperator), nil
	}

	// If the first rune is an '=' then it can be followed with an optional
	// '~' to match a regex
	l.accept("=")
	l.accept("~")
	return l.emit(TokenOperator), nil
}

func (l *Lexer) scanQuoted() (Token, error) {
	if err := l.expect("\""); err != nil {
		return Token{}, err
	}
	var isEscaped bool
	for r := l.next(); r != eof; r = l.next() {
		if isEscaped {
			isEscaped = false
		} else if r == '\\' {
			isEscaped = true
		} else if r == '"' {
			l.rewind()
			break
		}
	}
	if err := l.expect("\""); err != nil {
		return Token{}, UnterminatedError{
			input:       l.input,
			offsetStart: l.start,
			offsetEnd:   l.pos,
			columnStart: l.column,
			columnEnd:   l.cols,
			quote:       '"',
		}
	}
	return l.emit(TokenQuoted), nil
}

func (l *Lexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.rewind()
	return false
}

func (l *Lexer) acceptRun(valid string) {
	for strings.ContainsRune(valid, l.next()) {
	}
	l.rewind()
}

func (l *Lexer) expect(valid string) error {
	r := l.next()
	if r == -1 {
		l.rewind()
		return ExpectedError{
			input:       l.input,
			offsetStart: l.start,
			offsetEnd:   l.pos,
			columnStart: l.column,
			columnEnd:   l.cols,
			expected:    valid,
		}
	} else if !strings.ContainsRune(valid, r) {
		l.rewind()
		return ExpectedError{
			input:       l.input,
			offsetStart: l.start,
			offsetEnd:   l.pos,
			columnStart: l.column,
			columnEnd:   l.cols,
			expected:    valid,
		}
	} else {
		return nil
	}
}

func (l *Lexer) emit(kind TokenKind) Token {
	tok := Token{
		Kind:  kind,
		Value: l.input[l.start:l.pos],
		Position: Position{
			OffsetStart: l.start,
			OffsetEnd:   l.pos,
			ColumnStart: l.column,
			ColumnEnd:   l.cols,
		},
	}
	l.start = l.pos
	l.column = l.cols
	return tok
}

func (l *Lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	r, width := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = width
	l.pos += width
	l.cols++
	return r
}

func (l *Lexer) rewind() {
	if l.width > 0 {
		l.pos -= l.width
		l.width = 0
		l.cols--
	}
}

func (l *Lexer) skip() {
	l.start = l.pos
	l.column++
}
