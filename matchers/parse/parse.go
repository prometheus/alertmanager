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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	ErrEOF          = errors.New("end of input")
	ErrNoOpenBrace  = errors.New("expected opening brace")
	ErrNoCloseBrace = errors.New("expected close brace")
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
	// The final state of the parser, makes it idempotent.
	done     bool
	err      error
	matchers labels.Matchers

	input string
	lexer Lexer
	// Tracks if the input starts with a `{` and if we should expect a `}`.
	hasOpenParen bool
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

// Parse returns a series of matchers or an error. It is idempotent.
// Successive calls return the same result.
// once, however successive calls return the matchers and err from the first
// call.
func (p *Parser) Parse() (labels.Matchers, error) {
	if !p.done {
		p.done = true
		p.matchers, p.err = p.parse()
	}
	return p.matchers, p.err
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
	return Token{}, fmt.Errorf("%d:%d: unexpected %s", tok.ColumnStart, tok.ColumnEnd, tok.Value)
}

// peekNext peeks the next token from the lexer. It returns an error if there is
// no more input.
func (p *Parser) peekNext(l *Lexer) (Token, error) {
	tok, err := l.Peek()
	if err != nil {
		return Token{}, nil
	}
	if tok.Kind == TokenNone {
		return Token{}, fmt.Errorf("0:%d: %w", len(p.input), ErrEOF)
	}
	return tok, nil
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
	// Can start with an optional open brace.
	tok, err := p.peekNext(l)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return p.parseEOF, nil
		}
		return nil, err
	}
	p.hasOpenParen = tok.IsOneOf(TokenOpenBrace)
	// If the token was an open brace it must be scanned so the token
	// following it can be peeked.
	if p.hasOpenParen {
		if _, err = l.Scan(); err != nil {
			panic("Unexpected error scanning open brace")
		}

		// If the next token is a close brace there are no matchers in the input
		// and we can just parse the close brace.
		tok, err = p.peekNext(l)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", err, ErrNoCloseBrace)
		}
		if tok.IsOneOf(TokenCloseBrace) {
			return p.parseCloseParen, nil
		}
	}

	if tok.IsOneOf(TokenCloseBrace) {
		return p.parseCloseParen, nil
	}

	return p.parseLabelMatcher, nil
}

func (p *Parser) parseCloseParen(l *Lexer) (parseFn, error) {
	if p.hasOpenParen {
		// If there was an open brace there must be a matching close brace.
		if _, err := p.expect(l.Scan, TokenCloseBrace); err != nil {
			return nil, fmt.Errorf("%s: %w", err, ErrNoCloseBrace)
		}
	} else {
		// If there was no open brace there must not be a close brace either.
		if _, err := p.expect(l.Peek, TokenCloseBrace); err == nil {
			return nil, fmt.Errorf("0:%d: }: %w", len(p.input), ErrNoOpenBrace)
		}
	}
	return p.parseEOF, nil
}

func (p *Parser) parseComma(l *Lexer) (parseFn, error) {
	if _, err := p.expect(l.Scan, TokenComma); err != nil {
		return nil, fmt.Errorf("%s: %s", err, "expected a comma")
	}
	// The token after the comma can be another matcher, a close brace or the
	// end of input.
	tok, err := p.expect(l.Peek, TokenCloseBrace, TokenIdent, TokenQuoted)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			// If this is the end of input we still need to check if the optional
			// open brace has a matching close brace
			return p.parseCloseParen, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a matcher or close brace after comma")
	}
	if tok.Kind == TokenCloseBrace {
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
	// in double quotes.
	if tok, err = p.expect(l.Scan, TokenIdent, TokenQuoted); err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrNoLabelName)
	}
	labelName = tok.Value

	// The next token is the operator such as '=', '!=', '=~' and '!~'.
	if tok, err = p.expect(l.Scan, TokenOperator); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoOperator)
	}
	if ty, err = matchType(tok.Value); err != nil {
		panic("Unexpected operator")
	}

	// The next token is the label value. This too can either be an ident
	// which accepts just [a-zA-Z_] or a quoted which accepts all UTF-8
	// characters in double quotes.
	if tok, err = p.expect(l.Scan, TokenIdent, TokenQuoted); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoLabelValue)
	}
	if tok.Kind == TokenIdent {
		labelValue = tok.Value
	} else {
		labelValue, err = strconv.Unquote(tok.Value)
		if err != nil {
			return nil, fmt.Errorf("%d:%d: %s: invalid input", tok.ColumnStart, tok.ColumnEnd, tok.Value)
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
	tok, err := p.expect(l.Peek, TokenComma, TokenCloseBrace)
	if err != nil {
		// If this is the end of input we still need to check if the optional
		// open brace has a matching close brace.
		if errors.Is(err, ErrEOF) {
			return p.parseCloseParen, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a comma or close brace")
	}
	if tok.Kind == TokenCloseBrace {
		return p.parseCloseParen, nil
	} else if tok.Kind == TokenComma {
		return p.parseComma, nil
	} else {
		panic("unreachable")
	}
}

// Parse parses one or more matchers in the input string. It returns an error
// if the input is invalid.
func Parse(input string) (labels.Matchers, error) {
	p := NewParser(input)
	return p.Parse()
}

// ParseMatcher parses the matcher in the input string. It returns an error
// if the input is invalid or contains two or more matchers.
func ParseMatcher(input string) (*labels.Matcher, error) {
	if strings.HasPrefix(input, "{") {
		return nil, errors.New("Individual matchers cannot start and end with braces")
	}
	m, err := Parse(input)
	if err != nil {
		return nil, err
	}
	if len(m) > 1 {
		return nil, fmt.Errorf("expected 1 matcher, found %d", len(m))
	}
	return m[0], nil
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
