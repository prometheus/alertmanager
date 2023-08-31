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
	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	ErrEOF           = errors.New("end of input")
	ErrExpectedEOF   = errors.New("expected end of input")
	ErrUnexpectedEOF = func(p position) error { return fmt.Errorf("0:%d: %w", p.columnEnd, ErrEOF) }
	ErrNoOpenBrace   = errors.New("expected opening brace")
	ErrNoCloseBrace  = errors.New("expected close brace")
	ErrNoLabelName   = errors.New("expected label name")
	ErrNoLabelValue  = errors.New("expected label value")
	ErrNoOperator    = errors.New("expected an operator such as '=', '!=', '=~' or '!~'")
	ErrInvalidInput  = func(t token) error {
		return fmt.Errorf("%d:%d: %s: invalid input", t.columnStart, t.columnEnd, t.value)
	}
)

// Matchers parses one or more matchers in the input string. It returns an error
// if the input is invalid.
func Matchers(input string) (labels.Matchers, error) {
	p := parser{lexer: lexer{input: input}}
	return p.parse()
}

// Matcher parses the matcher in the input string. It returns an error
// if the input is invalid or contains two or more matchers.
func Matcher(input string) (*labels.Matcher, error) {
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
type parseFunc func(l *lexer) (parseFunc, error)

// parser reads the sequence of tokens from the lexer and returns either a
// series of matchers or an error. It works as a finite state automata, where
// each state in the automata is a parseFunc. The finite state automata can move
// from one state to another by returning the next parseFunc. It terminates when
// a parseFunc returns nil as the next parseFunc, if the lexer attempts to scan
// input that does not match the expected grammar, or if the tokens returned from
// the lexer cannot be parsed into a complete series of matchers.
type parser struct {
	matchers labels.Matchers

	// Tracks if the input starts with an open brace and if we should expect to
	// parse a close brace at the end of the input.
	hasOpenBrace bool
	lexer        lexer
}

func (p *parser) parse() (labels.Matchers, error) {
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

func (p *parser) parseOpenBrace(l *lexer) (parseFunc, error) {
	var (
		hasCloseBrace bool
		err           error
	)
	// Can start with an optional open brace.
	p.hasOpenBrace, err = p.accept(l, tokenOpenBrace)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return p.parseEOF, nil
		}
		return nil, err
	}
	// If the next token is a close brace there are no matchers in the input.
	hasCloseBrace, err = p.acceptPeek(l, tokenCloseBrace)
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

func (p *parser) parseCloseBrace(l *lexer) (parseFunc, error) {
	if p.hasOpenBrace {
		// If there was an open brace there must be a matching close brace.
		if _, err := p.expect(l, tokenCloseBrace); err != nil {
			return nil, fmt.Errorf("%s: %w", err, ErrNoCloseBrace)
		}
	} else {
		// If there was no open brace there must not be a close brace either.
		if _, err := p.expect(l, tokenCloseBrace); err == nil {
			pos := l.position()
			return nil, fmt.Errorf("0:%d: }: %w", pos.columnEnd, ErrNoOpenBrace)
		}
	}
	return p.parseEOF, nil
}

func (p *parser) parseMatcher(l *lexer) (parseFunc, error) {
	var (
		err                   error
		tok                   token
		matchName, matchValue string
		matchTy               labels.MatchType
	)
	// The first token should be the label name.
	if tok, err = p.expect(l, tokenQuoted, tokenUnquoted); err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrNoLabelName)
	}
	matchName, err = tok.unquote()
	if err != nil {
		return nil, ErrInvalidInput(tok)
	}
	// The next token should be the operator.
	if tok, err = p.expect(l, tokenEquals, tokenNotEquals, tokenMatches, tokenNotMatches); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoOperator)
	}
	switch tok.kind {
	case tokenEquals:
		matchTy = labels.MatchEqual
	case tokenNotEquals:
		matchTy = labels.MatchNotEqual
	case tokenMatches:
		matchTy = labels.MatchRegexp
	case tokenNotMatches:
		matchTy = labels.MatchNotRegexp
	default:
		return nil, errors.New("Unexpected operator, this should never happen")
	}
	// The next token should be the match value. Like the match name, this too
	// can be either double-quoted UTF-8 or unquoted UTF-8 without reserved characters.
	if tok, err = p.expect(l, tokenUnquoted, tokenQuoted); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoLabelValue)
	}
	matchValue, err = tok.unquote()
	if err != nil {
		return nil, ErrInvalidInput(tok)
	}
	m, err := labels.NewMatcher(matchTy, matchName, matchValue)
	if err != nil {
		return nil, fmt.Errorf("failed to create matcher: %s", err)
	}
	p.matchers = append(p.matchers, m)
	return p.parseEndOfMatcher, nil
}

func (p *parser) parseEndOfMatcher(l *lexer) (parseFunc, error) {
	tok, err := p.expectPeek(l, tokenComma, tokenCloseBrace)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			// If this is the end of input we still need to check if the optional
			// open brace has a matching close brace
			return p.parseCloseBrace, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a comma or close brace")
	}
	switch tok.kind {
	case tokenComma:
		return p.parseComma, nil
	case tokenCloseBrace:
		return p.parseCloseBrace, nil
	default:
		panic("Unexpected token at the end of matcher, this should never happen")
	}
}

func (p *parser) parseComma(l *lexer) (parseFunc, error) {
	if _, err := p.expect(l, tokenComma); err != nil {
		return nil, fmt.Errorf("%s: %s", err, "expected a comma")
	}
	// The token after the comma can be another matcher, a close brace or end of input.
	tok, err := p.expectPeek(l, tokenCloseBrace, tokenUnquoted, tokenQuoted)
	if err != nil {
		if errors.Is(err, ErrEOF) {
			// If this is the end of input we still need to check if the optional
			// open brace has a matching close brace
			return p.parseCloseBrace, nil
		}
		return nil, fmt.Errorf("%s: %s", err, "expected a matcher or close brace after comma")
	}
	if tok.kind == tokenCloseBrace {
		return p.parseCloseBrace, nil
	}
	return p.parseMatcher, nil
}

func (p *parser) parseEOF(l *lexer) (parseFunc, error) {
	tok, err := l.scan()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", err, ErrExpectedEOF)
	}
	if !tok.isEOF() {
		return nil, fmt.Errorf("%d:%d: %s: %w", tok.columnStart, tok.columnEnd, tok.value, ErrExpectedEOF)
	}
	return nil, nil
}

// accept returns true if the next token is one of the specified kinds,
// otherwise false. If the token is accepted it is consumed. tokenEOF is
// not an accepted kind  and instead accept returns ErrEOF if there is no
// more input.
func (p *parser) accept(l *lexer, kinds ...tokenKind) (ok bool, err error) {
	ok, err = p.acceptPeek(l, kinds...)
	if ok {
		if _, err = l.scan(); err != nil {
			panic("Failed to scan peeked token, this should never happen")
		}
	}
	return ok, err
}

// acceptPeek returns true if the next token is one of the specified kinds,
// otherwise false. However, unlike accept, acceptPeek does not consume accepted
// tokens. tokenEOF is not an accepted kind and instead accept returns ErrEOF
// if there is no more input.
func (p *parser) acceptPeek(l *lexer, kinds ...tokenKind) (bool, error) {
	tok, err := l.peek()
	if err != nil {
		return false, err
	}
	if tok.isEOF() {
		return false, ErrUnexpectedEOF(l.position())
	}
	return tok.isOneOf(kinds...), nil
}

// expect returns the next token if it is one of the specified kinds, otherwise
// it returns an error. If the token is expected it is consumed. tokenEOF is not
// an accepted kind and instead expect returns ErrEOF if there is no more input.
func (p *parser) expect(l *lexer, kind ...tokenKind) (token, error) {
	tok, err := p.expectPeek(l, kind...)
	if err != nil {
		return tok, err
	}
	if _, err = l.scan(); err != nil {
		panic("Failed to scan peeked token, this should never happen")
	}
	return tok, nil
}

// expect returns the next token if it is one of the specified kinds, otherwise
// it returns an error. However, unlike expect, expectPeek does not consume tokens.
// tokenEOF is not an accepted kind and instead expect returns ErrEOF if there is no
// more input.
func (p *parser) expectPeek(l *lexer, kind ...tokenKind) (token, error) {
	tok, err := l.peek()
	if err != nil {
		return tok, err
	}
	if tok.isEOF() {
		return tok, ErrUnexpectedEOF(l.position())
	}
	if !tok.isOneOf(kind...) {
		return tok, fmt.Errorf("%d:%d: unexpected %s", tok.columnStart, tok.columnEnd, tok.value)
	}
	return tok, nil
}
