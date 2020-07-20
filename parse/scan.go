package parse

import (
	"unicode"
	"unicode/utf8"
)

// eof rune sent when end of file is reached
var eof = rune(0)

// token is a lexical token.
type token uint

// list of lexical tokens.
const (
	// special tokens
	tokenIllegal token = iota
	tokenEOF

	// identifiers and literals
	tokenIdent

	// operators and delimiters
	tokenLbrack
	tokenRbrack
	tokenQuote
)

// predefined mode bits to control recognition of tokens.
const (
	scanIdent byte = 1 << iota
	scanLbrack
	scanRbrack
	scanEscape
)

// returns true if rune is accepted.
type acceptFunc func(r rune, i int) bool

// scanner implements a lexical scanner that reads unicode
// characters and tokens from a string buffer.
type scanner struct {
	buf   string
	pos   int
	start int
	width int
	mode  byte
	line  int

	accept acceptFunc
}

// init initializes a scanner with a new buffer.
func (s *scanner) init(buf string) {
	s.buf = buf
	s.pos = 0
	s.start = 0
	s.width = 0
	s.line = 1
	s.accept = nil
}

// read returns the next unicode character. It returns eof at
// the end of the string buffer.
// NOTE: Maybe we can store the context on the scanner as a windowing operation
// with certain before and after characters so that it can be retrieved for
// printing out in error cases
func (s *scanner) read() rune {
	if s.pos >= len(s.buf) {
		s.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(s.buf[s.pos:])
	s.width = w
	s.pos += s.width
	if r == '\n' {
		println("found new line")
		s.line++
	}
	return r
}

func (s *scanner) unread() {
	s.pos -= s.width
}

// skip skips over the curring unicode character in the buffer
// by slicing and removing from the buffer.
func (s *scanner) skip() {
	l := s.buf[:s.pos-1]
	r := s.buf[s.pos:]
	s.buf = l + r
}

// peek returns the next unicode character in the buffer without
// advancing the scanner. It returns eof if the scanner's position
// is at the last character of the source.
func (s *scanner) peek() rune {
	ln := s.line
	r := s.read()
	// if we increment the line number on read, make sure we decrement it by
	// the same amount since we are only peeking.
	if s.line != ln {
		s.line -= (s.line - ln)
	}
	s.unread()
	return r
}

// string returns the string corresponding to the most recently
// scanned token. Valid after calling scan().
func (s *scanner) string() string {
	return s.buf[s.start:s.pos]
}

// context returns the context around the most recently scanned token. Valid
// after calling scan(). The context length is 10 characters.
func (s *scanner) context() string {
	contextLen := 10
	st := s.start
	p := s.pos
	if s.start-contextLen > 0 {
		st = s.start - contextLen
	}
	if s.pos+contextLen < len(s.buf) {
		p = s.pos + contextLen
	}
	return s.buf[st:p]
}

// scan reads the next token or Unicode character from source and
// returns it. It returns EOF at the end of the source.
// TODO: scan may have to return the token and the rune, so that we can print
// out the rune for error msging
func (s *scanner) scan() token {
	s.start = s.pos
	r := s.read()
	switch {
	case r == eof:
		return tokenEOF
	case s.scanLbrack(r):
		return tokenLbrack
	case s.scanRbrack(r):
		return tokenRbrack
	case s.scanIdent(r):
		return tokenIdent
	}
	// print the rune that was read
	// log.Printf("RUNE BEFORE ILLEGAL: %c\n", r)
	return tokenIllegal
}

// scanIdent reads the next token or Unicode character from source
// and returns true if the Ident character is accepted.
func (s *scanner) scanIdent(r rune) bool {
	if s.mode&scanIdent == 0 {
		return false
	}
	if s.scanEscaped(r) {
		s.skip()
	} else if !s.accept(r, s.pos-s.start) {
		return false
	}
loop:
	for {
		r := s.read()
		switch {
		case r == eof:
			s.unread()
			break loop
		case s.scanLbrack(r):
			s.unread()
			s.unread()
			break loop
		}
		if s.scanEscaped(r) {
			s.skip()
			continue
		}
		if !s.accept(r, s.pos-s.start) {
			s.unread()
			break loop
		}
	}
	return true
}

// scanLbrack reads the next token or Unicode character from source
// and returns true if the open bracket is encountered.
func (s *scanner) scanLbrack(r rune) bool {
	if s.mode&scanLbrack == 0 {
		return false
	}
	if r == '$' {
		if s.read() == '{' {
			return true
		}
		s.unread()
	}
	return false
}

// scanRbrack reads the next token or Unicode character from source
// and returns true if the closing bracket is encountered.
func (s *scanner) scanRbrack(r rune) bool {
	if s.mode&scanRbrack == 0 {
		return false
	}
	return r == '}'
}

// scanEscaped reads the next token or Unicode character from source
// and returns true if it being escaped and should be sipped.
func (s *scanner) scanEscaped(r rune) bool {
	if s.mode&scanEscape == 0 {
		return false
	}
	if r == '$' {
		if s.peek() == '$' {
			return true
		}
	}
	if r != '\\' {
		return false
	}
	switch s.peek() {
	case '/', '\\':
		return true
	default:
		return false
	}
}

//
// scanner functions accept or reject runes.
//

func acceptRune(r rune, i int) bool {
	return true
}

func acceptIdent(r rune, i int) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func acceptColon(r rune, i int) bool {
	return r == ':'
}

func acceptOneHash(r rune, i int) bool {
	return r == '#' && i == 1
}

func acceptNone(r rune, i int) bool {
	return false
}

func acceptNotClosing(r rune, i int) bool {
	return r != '}'
}

func acceptHashFunc(r rune, i int) bool {
	return r == '#' && i < 3
}

func acceptPercentFunc(r rune, i int) bool {
	return r == '%' && i < 3
}

func acceptDefaultFunc(r rune, i int) bool {
	switch {
	case i == 1 && r == ':':
		return true
	case i == 2 && (r == '=' || r == '-' || r == '?' || r == '+'):
		return true
	default:
		return false
	}
}

func acceptReplaceFunc(r rune, i int) bool {
	switch {
	case i == 1 && r == '/':
		return true
	case i == 2 && (r == '/' || r == '#' || r == '%'):
		return true
	default:
		return false
	}
}

func acceptOneEqual(r rune, i int) bool {
	return i == 1 && r == '='
}

func acceptOneColon(r rune, i int) bool {
	return i == 1 && r == ':'
}

func rejectColonClose(r rune, i int) bool {
	return r != ':' && r != '}'
}

func acceptSlash(r rune, i int) bool {
	return r == '/'
}

func acceptNotSlash(r rune, i int) bool {
	return r != '/'
}

func acceptCasingFunc(r rune, i int) bool {
	return (r == ',' || r == '^') && i < 3
}
