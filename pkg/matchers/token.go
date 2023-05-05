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
