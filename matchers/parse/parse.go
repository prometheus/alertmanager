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
	ErrEOF           = errors.New("end of input")
	ErrNoOpenBrace   = errors.New("expected opening brace")
	ErrNoCloseBrace  = errors.New("expected close brace")
	ErrNoLabelName   = errors.New("expected label name")
	ErrNoLabelValue  = errors.New("expected label value")
	ErrNoOperator    = errors.New("expected an operator such as '=', '!=', '=~' or '!~'")
	ErrExpectedEOF   = errors.New("expected end of input")
	ErrUnexpectedEOF = func(l *Lexer) error { return fmt.Errorf("0:%d: %w", l.Pos().ColumnEnd, ErrEOF) }
	ErrUnquotable    = func(t Token) error {
		return fmt.Errorf("%d:%d: %s: invalid input", t.ColumnStart, t.ColumnEnd, t.Value)
	}
	ErrInvalidQuoted = func(t Token) error {
		return fmt.Errorf("%d:%d: %s: invalid input", t.ColumnStart, t.ColumnEnd, t.Value)
	}
)

// Matchers parses one or more matchers in the input string. It returns an error
// if the input is invalid.
func Matchers(input string) (labels.Matchers, error) {
	p := Parser{lexer: Lexer{input: input}}
	return p.Parse()
}

// Matcher parses the matcher in the input string. It returns an error
// if the input is invalid or contains two or more matchers.
func Matcher(input string) (*labels.Matcher, error) {
	if strings.HasPrefix(input, "{") || strings.HasSuffix(input, "}") {
		return nil, errors.New("matcher cannot start or end with braces")
	}
	m, err := Matchers(input)
	if err != nil {
		return nil, err
	}
	switch len(m) {
	case 1:
		return m[0], nil
	case 0:
		return nil, fmt.Errorf("no matchers")
	default:
		return nil, fmt.Errorf("expected 1 matcher, found %d", len(m))
	}
}

// parseFunc is state in the finite state automata.
type parseFunc func(l *Lexer) (parseFunc, error)

// Parser reads the sequence of tokens from the Lexer and returns either a
// series of matchers or an error. It works as a finite state automata, where
// each state in the automata is a parseFunc. The finite state automata can move
// from one state to another by returning the next parseFunc. It terminates when
// a parseFunc returns nil as the next parseFunc, if the Lexer attempts to Scan
// input that does not match the expected grammar, or if the tokens returned from
// the Lexer cannot be parsed into a complete series of matchers.
type Parser struct {
	// The final state of the Parser, makes it idempotent.
	matchers labels.Matchers

	// Tracks if the input starts with an open brace and if we should expect to
	// parse a close brace at the end of the input.
	hasOpenBrace bool
	lexer        Lexer
}

func (p *Parser) Parse() (labels.Matchers, error) {
	var (
		err error
		fn  = p.parseOpenBrace
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

func (p *Parser) parseOpenBrace(l *Lexer) (parseFunc, error) {
	var (
		hasCloseBrace bool
		err           error
	)
	// Can start with an optional open brace.
	p.hasOpenBrace, err = p.accept(l, TokenOpenBrace)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return p.parseEOF, nil
		}
		return nil, err
	}
	// If the next token is a close brace there are no matchers in the input.
	hasCloseBrace, err = p.acceptPeek(l, TokenCloseBrace)
	if err != nil {
		// If there is no more input after the open brace then parse the close brace
		// so the error message contains ErrNoCloseBrace.
		if errors.Is(err, ErrEOF) {
			return p.parseCloseBrace, nil
		}
		return nil, err
	}
	if hasCloseBrace {
		return p.parseCloseBrace, nil
	}
	return p.parseMatcher, nil
}

func (p *Parser) parseCloseBrace(l *Lexer) (parseFunc, error) {
	if p.hasOpenBrace {
		// If there was an open brace there must be a matching close brace.
		if _, err := p.expect(l, TokenCloseBrace); err != nil {
			return nil, fmt.Errorf("%s: %w", err, ErrNoCloseBrace)
		}
	} else {
		// If there was no open brace there must not be a close brace either.
		if _, err := p.expect(l, TokenCloseBrace); err == nil {
			pos := l.Pos()
			return nil, fmt.Errorf("0:%d: }: %w", pos.ColumnEnd, ErrNoOpenBrace)
		}
	}
	return p.parseEOF, nil
}

func (p *Parser) parseMatcher(l *Lexer) (parseFunc, error) {
	var (
		err                   error
		tok                   Token
		matchName, matchValue string
		matchTy               labels.MatchType
	)
	// The first token should be the label name.
	if tok, err = p.expect(l, TokenQuoted, TokenUnquoted); err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrNoLabelName)
	}
	matchName, err = p.unquote(tok)
	if err != nil {
		return nil, err
	}
	// The next token should be the operator.
	if tok, err = p.expect(l, TokenEquals, TokenNotEquals, TokenMatches, TokenNotMatches); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoOperator)
	}
	switch tok.Kind {
	case TokenEquals:
		matchTy = labels.MatchEqual
	case TokenNotEquals:
		matchTy = labels.MatchNotEqual
	case TokenMatches:
		matchTy = labels.MatchRegexp
	case TokenNotMatches:
		matchTy = labels.MatchNotRegexp
	default:
		return nil, errors.New("Unexpected operator, this should never happen")
	}
	// The next token should be the match value. Like the match name, this too
	// can be either double-quoted UTF-8 or unquoted UTF-8 without reserved characters.
	if tok, err = p.expect(l, TokenUnquoted, TokenQuoted); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoLabelValue)
	}
	matchValue, err = p.unquote(tok)
	if err != nil {
		return nil, err
	}
	m, err := labels.NewMatcher(matchTy, matchName, matchValue)
	if err != nil {
		return nil, fmt.Errorf("failed to create matcher: %s", err)
	}
	p.matchers = append(p.matchers, m)
	return p.parseEndOfMatcher, nil
}

func (p *Parser) parseEndOfMatcher(l *Lexer) (parseFunc, error) {
	tok, err := p.expectPeek(l, TokenComma, TokenCloseBrace)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			// If this is the end of input we still need to check if the optional
			// open brace has a matching close brace
			return p.parseCloseBrace, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a comma or close brace")
	}
	switch tok.Kind {
	case TokenComma:
		return p.parseComma, nil
	case TokenCloseBrace:
		return p.parseCloseBrace, nil
	default:
		panic("Unexpected token at the end of matcher, this should never happen")
	}
}

func (p *Parser) parseComma(l *Lexer) (parseFunc, error) {
	if _, err := p.expect(l, TokenComma); err != nil {
		return nil, fmt.Errorf("%s: %s", err, "expected a comma")
	}
	// The token after the comma can be another matcher, a close brace or end of input.
	tok, err := p.expectPeek(l, TokenCloseBrace, TokenUnquoted, TokenQuoted)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			// If this is the end of input we still need to check if the optional
			// open brace has a matching close brace
			return p.parseCloseBrace, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a matcher or close brace after comma")
	}
	if tok.Kind == TokenCloseBrace {
		return p.parseCloseBrace, nil
	}
	return p.parseMatcher, nil
}

func (p *Parser) parseEOF(l *Lexer) (parseFunc, error) {
	tok, err := l.Scan()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrExpectedEOF)
	}
	if !tok.IsEOF() {
		return nil, fmt.Errorf("%d:%d: %s: %w", tok.ColumnStart, tok.ColumnEnd, tok.Value, ErrExpectedEOF)
	}
	return nil, nil
}

// accept returns true if the next token is one of the specified kinds,
// otherwise false. If the token is accepted it is consumed. TokenEOF is
// not an accepted kind  and instead accept returns ErrEOF if there is no
// more input.
func (p *Parser) accept(l *Lexer, kinds ...TokenKind) (ok bool, err error) {
	ok, err = p.acceptPeek(l, kinds...)
	if ok {
		if _, err = l.Scan(); err != nil {
			panic("Failed to Scan peeked token, this should never happen")
		}
	}
	return ok, err
}

// acceptPeek returns true if the next token is one of the specified kinds,
// otherwise false. However, unlike accept, acceptPeek does not consume accepted
// tokens. TokenEOF is not an accepted kind and instead accept returns ErrEOF
// if there is no more input.
func (p *Parser) acceptPeek(l *Lexer, kinds ...TokenKind) (bool, error) {
	tok, err := l.Peek()
	if err != nil {
		return false, err
	}
	if tok.IsEOF() {
		return false, ErrUnexpectedEOF(l)
	}
	return tok.IsOneOf(kinds...), nil
}

// expect returns the next token if it is one of the specified kinds, otherwise
// it returns an error. If the token is expected it is consumed. TokenEOF is not
// an accepted kind and instead expect returns ErrEOF if there is no more input.
func (p *Parser) expect(l *Lexer, kind ...TokenKind) (Token, error) {
	tok, err := p.expectPeek(l, kind...)
	if err != nil {
		return tok, err
	}
	if _, err = l.Scan(); err != nil {
		panic("Failed to Scan peeked token, this should never happen")
	}
	return tok, nil
}

// expect returns the next token if it is one of the specified kinds, otherwise
// it returns an error. However, unlike expect, expectPeek does not consume tokens.
// TokenEOF is not an accepted kind and instead expect returns ErrEOF if there is no
// more input.
func (p *Parser) expectPeek(l *Lexer, kind ...TokenKind) (Token, error) {
	tok, err := l.Peek()
	if err != nil {
		return tok, err
	}
	if tok.IsEOF() {
		return tok, ErrUnexpectedEOF(l)
	}
	if !tok.IsOneOf(kind...) {
		return tok, fmt.Errorf("%d:%d: unexpected %s", tok.ColumnStart, tok.ColumnEnd, tok.Value)
	}
	return tok, nil
}

// Unquote the value in token. If unquoted returns it unmodified.
func (p *Parser) unquote(t Token) (string, error) {
	if t.Kind == TokenQuoted {
		s, err := strconv.Unquote(t.Value)
		if err != nil {
			return "", ErrInvalidQuoted(t)
		}
		return s, nil
	}
	return t.Value, nil
}
