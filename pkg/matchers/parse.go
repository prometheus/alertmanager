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
	"errors"
	"fmt"
	"strconv"

	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	ErrEOF          = errors.New("end of input")
	ErrNoOpenParen  = errors.New("expected opening paren")
	ErrNoCloseParen = errors.New("expected close paren")
	ErrNoLabelName  = errors.New("expected label name")
	ErrNoLabelValue = errors.New("expected label value")
	ErrNoOperator   = errors.New("expected an operator such as '=', '!=', '=~' or '!~'")
	ErrExpectedEOF  = errors.New("expected end of input")
)

// Parser reads the sequence of tokens from the lexer and returns either a
// series of matchers or an error. An error can occur if the lexer attempts
// to scan text that does not match the expected grammar, or if the tokens
// returned from the lexer cannot be parsed into a complete series of matchers.
// For example, the input is missing an opening bracket, has missing label
// names or label values, a trailing comma, or missing closing bracket.
type Parser struct {
	done         bool
	err          error
	hasOpenParen bool
	input        string
	lexer        Lexer
	matchers     labels.Matchers
}

func NewParser(input string) Parser {
	return Parser{
		input: input,
		lexer: NewLexer(input),
	}
}

// Error returns the error that caused parsing to fail.
func (p *Parser) Error() error {
	return p.err
}

// Parse returns a series of matchers or an error. It can be called more than
// once, however successive calls return the matchers and err from the first
// call.
func (p *Parser) Parse() (labels.Matchers, error) {
	if !p.done {
		p.done = true
		p.matchers, p.err = p.parse()
	}
	return p.matchers, p.err
}

// accept returns true if the next token is one of the expected kinds, or
// an error if the next token that would be returned from the lexer does not
// match the expected grammar, or if the lexer has reached the end of the input
// and TokenNone is not one of the accepted kinds. It is possible to use either
// Scan() or Peek() as fn depending on whether accept should consume or peek
// the next token.
func (p *Parser) accept(fn func() (Token, error), kind ...TokenKind) (bool, error) {
	var (
		err error
		tok Token
	)
	if tok, err = fn(); err != nil {
		return false, err
	}
	for _, k := range kind {
		if tok.Kind == k {
			return true, nil
		}
	}
	if tok.Kind == TokenNone {
		return false, fmt.Errorf("0:%d: %w", len(p.input), ErrEOF)
	}
	return false, nil
}

// expect returns the next token if it is one of the expected kinds. It returns
// an error if the next token that would be returned from the lexer does not
// match the expected grammar, or if the lexer has reached the end of the input
// and TokenNone is not one of the expected kinds. It is possible to use either
// Scan() or Peek() as fn depending on whether expect should consume or peek
// the next token.
func (p *Parser) expect(fn func() (Token, error), kind ...TokenKind) (Token, error) {
	var (
		err error
		tok Token
	)
	if tok, err = fn(); err != nil {
		return Token{}, err
	}
	for _, k := range kind {
		if tok.Kind == k {
			return tok, nil
		}
	}
	if tok.Kind == TokenNone {
		return Token{}, fmt.Errorf("0:%d: %w", len(p.input), ErrEOF)
	}
	return Token{}, fmt.Errorf("%d:%d: unexpected %s", tok.Start, tok.End, tok.Value)
}

func (p *Parser) parse() (labels.Matchers, error) {
	var (
		err error
		fn  = p.parseOpenParen
		l   = &p.lexer
	)
	for {
		if fn, err = fn(l); err != nil {
			return nil, err
		} else if fn == nil {
			break
		}
	}
	return p.matchers, nil
}

type parseFn func(l *Lexer) (parseFn, error)

func (p *Parser) parseOpenParen(l *Lexer) (parseFn, error) {
	// Can start with an optional open paren
	hasOpenParen, err := p.accept(l.Peek, TokenOpenParen)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return p.parseEOF, nil
		}
		return nil, err
	}
	if hasOpenParen {
		// If the token was an open paren it must be scanned so the token
		// following it can be peeked
		if _, err = l.Scan(); err != nil {
			panic("Unexpected error scanning open paren")
		}
	}
	p.hasOpenParen = hasOpenParen
	// If the next token is a close paren there are no matchers in the input
	// and we can just parse the close paren
	if hasCloseParen, err := p.accept(l.Peek, TokenCloseParen); err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrNoCloseParen)
	} else if hasCloseParen {
		return p.parseCloseParen, nil
	}
	return p.parseLabelMatcher, nil
}

func (p *Parser) parseCloseParen(l *Lexer) (parseFn, error) {
	if p.hasOpenParen {
		// If there was an open paren there must be a matching close paren
		if _, err := p.expect(l.Scan, TokenCloseParen); err != nil {
			return nil, fmt.Errorf("%s: %w", err, ErrNoCloseParen)
		}
	} else {
		// If there was no open paren there must not be a close paren either
		if _, err := p.expect(l.Peek, TokenCloseParen); err == nil {
			return nil, fmt.Errorf("0:%d: }: %w", len(p.input), ErrNoOpenParen)
		}
	}
	return p.parseEOF, nil
}

func (p *Parser) parseComma(l *Lexer) (parseFn, error) {
	if _, err := p.expect(l.Scan, TokenComma); err != nil {
		return nil, fmt.Errorf("%s: %s", err, "expected a comma")
	}
	// The token after the comma can be another matcher, a close paren or the
	// end of input
	tok, err := p.expect(l.Peek, TokenCloseParen, TokenIdent, TokenQuoted)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			// If this is the end of input we still need to check if the optional
			// open paren has a matching close paren
			return p.parseCloseParen, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a matcher or close paren after comma")
	}
	if tok.Kind == TokenCloseParen {
		return p.parseCloseParen, nil
	}
	return p.parseLabelMatcher, nil
}

func (p *Parser) parseEOF(l *Lexer) (parseFn, error) {
	if _, err := p.expect(l.Scan, TokenNone); err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrExpectedEOF)
	}
	return nil, nil
}

func (p *Parser) parseLabelMatcher(l *Lexer) (parseFn, error) {
	var (
		err        error
		tok        Token
		labelName  string
		labelValue string
		ty         labels.MatchType
	)

	// The next token is the label name. This can either be an ident which
	// accepts just [a-zA-Z_] or a quoted which accepts all UTF-8 characters
	// in double quotes
	if tok, err = p.expect(l.Scan, TokenIdent, TokenQuoted); err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrNoLabelName)
	}
	labelName = tok.Value

	// The next token is the operator such as '=', '!=', '=~' and '!~'
	if tok, err = p.expect(l.Scan, TokenOperator); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoOperator)
	}
	if ty, err = matchType(tok.Value); err != nil {
		panic("Unexpected operator")
	}

	// The next token is the label value. This too can either be an ident
	// which accepts just [a-zA-Z_] or a quoted which accepts all UTF-8
	// characters in double quotes
	if tok, err = p.expect(l.Scan, TokenIdent, TokenQuoted); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoLabelValue)
	}
	if tok.Kind == TokenIdent {
		labelValue = tok.Value
	} else {
		labelValue, err = strconv.Unquote(tok.Value)
		if err != nil {
			return nil, fmt.Errorf("%d:%d: %s: invalid input", tok.Start, tok.End, tok.Value)
		}
	}

	m, err := labels.NewMatcher(ty, labelName, labelValue)
	if err != nil {
		return nil, fmt.Errorf("failed to create matcher: %s", err)
	}
	p.matchers = append(p.matchers, m)

	return p.parseLabelMatcherEnd, nil
}

func (p *Parser) parseLabelMatcherEnd(l *Lexer) (parseFn, error) {
	tok, err := p.expect(l.Peek, TokenComma, TokenCloseParen)
	if err != nil {
		// If this is the end of input we still need to check if the optional
		// open paren has a matching close paren
		if errors.Is(err, ErrEOF) {
			return p.parseCloseParen, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a comma or close paren")
	}
	if tok.Kind == TokenCloseParen {
		return p.parseCloseParen, nil
	} else if tok.Kind == TokenComma {
		return p.parseComma, nil
	} else {
		panic("unreachable")
	}
}

func Parse(input string) (labels.Matchers, error) {
	p := NewParser(input)
	return p.Parse()
}

func matchType(s string) (labels.MatchType, error) {
	switch s {
	case "=":
		return labels.MatchEqual, nil
	case "!=":
		return labels.MatchNotEqual, nil
	case "=~":
		return labels.MatchRegexp, nil
	case "!~":
		return labels.MatchNotRegexp, nil
	default:
		return -1, fmt.Errorf("unexpected operator: %s", s)
	}
}
