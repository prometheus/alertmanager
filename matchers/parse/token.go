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
	"strconv"
)

type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenOpenBrace
	TokenCloseBrace
	TokenComma
	TokenEquals
	TokenNotEquals
	TokenMatches
	TokenNotMatches
	TokenQuoted
	TokenUnquoted
)

func (k TokenKind) String() string {
	switch k {
	case TokenOpenBrace:
		return "OpenBrace"
	case TokenCloseBrace:
		return "CloseBrace"
	case TokenComma:
		return "Comma"
	case TokenEquals:
		return "Equals"
	case TokenNotEquals:
		return "NotEquals"
	case TokenMatches:
		return "Matches"
	case TokenNotMatches:
		return "NotMatches"
	case TokenQuoted:
		return "Quoted"
	case TokenUnquoted:
		return "Unquoted"
	default:
		return "EOF"
	}
}

type Token struct {
	Kind  TokenKind
	Value string
	Position
}

// IsEOF returns true if the token is an end of file token.
func (t Token) IsEOF() bool {
	return t.Kind == TokenEOF
}

// IsOneOf returns true if the token is one of the specified kinds.
func (t Token) IsOneOf(kinds ...TokenKind) bool {
	for _, k := range kinds {
		if k == t.Kind {
			return true
		}
	}
	return false
}

// Unquote the value in token. If unquoted returns it unmodified.
func (t Token) Unquote() (string, error) {
	if t.Kind == TokenQuoted {
		return strconv.Unquote(t.Value)
	}
	return t.Value, nil
}

func (t Token) String() string {
	return fmt.Sprintf("(%s) '%s'", t.Kind, t.Value)
}

type Position struct {
	OffsetStart int // The start position in the input.
	OffsetEnd   int // The end position in the input.
	ColumnStart int // The column number.
	ColumnEnd   int // The end of the column.
}
