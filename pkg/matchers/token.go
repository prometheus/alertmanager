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
	"fmt"
)

type TokenKind int

const (
	TokenNone TokenKind = iota
	TokenCloseParen
	TokenComma
	TokenIdent
	TokenOpenParen
	TokenOperator
	TokenQuoted
)

func (k TokenKind) String() string {
	switch k {
	case TokenCloseParen:
		return "CloseParen"
	case TokenComma:
		return "Comma"
	case TokenIdent:
		return "Ident"
	case TokenOpenParen:
		return "OpenParen"
	case TokenOperator:
		return "Op"
	case TokenQuoted:
		return "Quoted"
	default:
		return "None"
	}
}

type Token struct {
	Kind  TokenKind
	Value string
	Start int // The start position in the input
	End   int // The end position in the input
}

func (t Token) String() string {
	return fmt.Sprintf("(%s) '%s'", t.Kind, t.Value)
}

func IsNone(t Token) bool {
	return t == Token{}
}
