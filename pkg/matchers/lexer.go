package matchers

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

// InvalidInputError is returned when the next rune in the input does not match
// the grammar of Prometheus-like matchers.
type InvalidInputError struct {
	input string
	start int
	end   int
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("%d:%d: %s: invalid input", e.start, e.end, e.input[e.start:e.end])
}

// Lexer scans a sequence of tokens that match the grammar of Prometheus-like
// matchers. A token is emitted for each call to Scan() which returns the
// next token in the input or an error if the input does not conform to the
// grammar. A token can be one of a number of kinds and corresponds to a
// subslice of the input. Once the input has been consumed successive calls to
// Scan() return a TokenNone token.
type Lexer struct {
	input string
	err   error
	start int // the start of the current token
	pos   int // the position of the cursor in the input
	width int // the width of the last rune
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
	// Do not reset l.err because we can return it on the next call to Scan()
	defer func() {
		l.start = start
		l.pos = pos
		l.width = width
	}()
	return l.Scan()
}

func (l *Lexer) Scan() (Token, error) {
	// Do not attempt to emit more tokens if the input is invalid
	if l.err != nil {
		return Token{}, l.err
	}

	// Iterate over each rune in the input and either emit a token or an error
	for r := l.next(); r != eof; r = l.next() {
		switch {
		case r == '{':
			return l.emit(TokenOpenParen), nil
		case r == '}':
			return l.emit(TokenCloseParen), nil
		case r == ',':
			return l.emit(TokenComma), nil
		case r == '=' || r == '!':
			l.rewind()
			var tok Token
			tok, l.err = l.scanOperator()
			return tok, l.err
		case r == '"':
			l.rewind()
			var tok Token
			tok, l.err = l.scanQuoted()
			return tok, l.err
		case r == '_' || isAlpha(r):
			l.rewind()
			var tok Token
			tok, l.err = l.scanIdent()
			return tok, l.err
		case unicode.IsSpace(r):
			l.skip()
		default:
			l.err = InvalidInputError{input: l.input, start: l.start, end: l.pos}
			return Token{}, l.err
		}
	}

	return Token{}, nil
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
		if r == '\\' {
			isEscaped = true
		} else if r == '"' && !isEscaped {
			l.rewind()
			break
		} else {
			isEscaped = false
		}
	}

	if err := l.expect("\""); err != nil {
		return Token{}, err
	}

	return l.emit(TokenQuoted), nil
}

func (l *Lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.rewind()
	return false
}

func (l *Lexer) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
	}
	l.rewind()
}

func (l *Lexer) expect(valid string) error {
	r := l.next()
	if r == -1 {
		l.rewind()
		return fmt.Errorf("%d:%d: expected one of '%s', got EOF", l.start, l.pos, valid)
	} else if strings.IndexRune(valid, r) < 0 {
		l.rewind()
		return fmt.Errorf("%d:%d: expected one of '%s', got '%c'", l.start, l.pos, valid, r)
	} else {
		return nil
	}
}

func (l *Lexer) emit(kind TokenKind) Token {
	tok := Token{
		Kind:  kind,
		Value: l.input[l.start:l.pos],
		Start: l.start,
		End:   l.pos,
	}
	l.start = l.pos
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
	return r
}

func (l *Lexer) rewind() {
	l.pos -= l.width
	l.width = 0
}

func (l *Lexer) skip() {
	l.start = l.pos
}
