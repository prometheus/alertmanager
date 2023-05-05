package matchers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	ErrEOF                 = errors.New("EOF")
	ErrNoCloseParen        = errors.New("expected closing '}'")
	ErrorNoCommaCloseParen = errors.New("expected comma or closing '}'")
	ErrNoLabelName         = errors.New("expected label name")
	ErrNoLabelValue        = errors.New("expected label value")
	ErrNoOpenParen         = errors.New("expected opening '{'")
	ErrNoOperator          = errors.New("expected an operator such as '=', '!=', '=~' or '!~'")
)

// Parser reads the sequence of tokens from the lexer and returns either a
// series of matchers or an error. An error can occur if the lexer attempts
// to scan text that does not match the expected grammar, or if the tokens
// returned from the lexer cannot be parsed into a complete series of matchers.
// For example, the input is missing an opening bracket, has missing label
// names or label values, a trailing comma, or missing closing bracket.
type Parser struct {
	done     bool
	err      error
	input    string
	lexer    Lexer
	matchers labels.Matchers
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
	tok, err := fn()
	if err != nil {
		return false, err
	}
	for _, k := range kind {
		if tok.Kind == k {
			return true, nil
		}
	}
	if tok.Kind == TokenNone {
		return false, ErrEOF
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
	tok, err := fn()
	if err != nil {
		return Token{}, err
	}
	for _, k := range kind {
		if tok.Kind == k {
			return tok, nil
		}
	}
	if tok.Kind == TokenNone {
		return Token{}, ErrEOF
	}
	return Token{}, fmt.Errorf("%d:%d: unexpected %s", tok.Start, tok.End, tok.Value)
}

func (p *Parser) parse() (labels.Matchers, error) {
	var (
		err error
		tok Token
	)

	l := &p.lexer

	// Must start with opening paren
	if tok, err = p.expect(l.Scan, TokenOpenParen); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoOpenParen)
	}

	for {
		// Break if there is a closing paren
		if ok, _ := p.accept(l.Peek, TokenCloseParen); ok {
			break
		}

		// The next token is the label name. This can either be an ident which
		// accepts just [a-zA-Z_] or a quoted which accepts all UTF-8 characters
		// in double quotes.
		if tok, err = p.expect(l.Scan, TokenIdent, TokenQuoted); err != nil {
			return nil, fmt.Errorf("%s: %s", err, ErrNoLabelName)
		}
		name := tok.Value

		// The next token is the operator such as '=', '!=', '=~' and '!~'
		if tok, err = p.expect(l.Scan, TokenOperator); err != nil {
			return nil, fmt.Errorf("%s: %s", err, ErrNoOperator)
		}
		op, err := operatorFromString(tok.Value)
		if err != nil {
			// This should never happen because operators are checked against
			// the grammar.
			panic("Unexpected operator")
		}

		// The next token is the label value. This too can either be an ident
		// which accepts just [a-zA-Z_] or a quoted which accepts all UTF-8
		// characters in double quotes.
		if tok, err = p.expect(l.Scan, TokenIdent, TokenQuoted); err != nil {
			return nil, fmt.Errorf("%s: %s", err, ErrNoLabelValue)
		}
		value := strings.TrimPrefix(strings.TrimSuffix(tok.Value, "\""), "\"")

		m, err := labels.NewMatcher(op, name, value)
		if err != nil {
			return nil, fmt.Errorf("failed to create matcher: %s", err)
		}
		p.matchers = append(p.matchers, m)

		// The next token should be either a comma or a closing paren.
		if tok, err = p.expect(l.Peek, TokenComma, TokenCloseParen); err != nil {
			return nil, fmt.Errorf("%s: %s", err, ErrorNoCommaCloseParen)
		} else if tok.Kind == TokenComma {
			// The next token is a comma, and so we expect to parse more matchers.
			// That means the next token must be a label name.
			if tok, err = l.Scan(); err != nil {
				panic("Unexpected error scanning peeked comma")
			}
			if tok, err = p.expect(l.Peek, TokenIdent, TokenQuoted); err != nil {
				return nil, fmt.Errorf("%s: %s", err, ErrNoLabelName)
			}
		}
	}

	if tok, err = p.expect(l.Scan, TokenCloseParen); err != nil {
		return nil, fmt.Errorf("%s: %s", err, ErrNoCloseParen)
	}

	// There should be no more tokens.
	if tok, err = p.expect(l.Scan, TokenNone); err != nil {
		return nil, fmt.Errorf("%s: %s", err, "")
	}

	return p.matchers, nil
}

func Parse(input string) (labels.Matchers, error) {
	p := NewParser(input)
	return p.Parse()
}

func operatorFromString(s string) (labels.MatchType, error) {
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
