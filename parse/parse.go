package parse

import (
	"errors"
	"fmt"
	"log"
)

// ErrBadSubstitution represents a substitution parsing error.
var ErrBadSubstitution = errors.New("bad substitution")

type ErrParse struct {
	lineNumber int
	context    string
	err        error
}

func (e *ErrParse) Error() string {
	if e.lineNumber > 0 {
		return fmt.Sprintf("%s on line %d\n\tLook for:\"...%s...\"\n", e.err, e.lineNumber, e.context)
	}
	return fmt.Sprintf("%s\n\tLook for:\"...%s...\"\n", e.err, e.context)
}

// Tree is the representation of a single parsed SQL statement.
type Tree struct {
	Root    Node
	Context string

	// Parsing only; cleared after parse.
	scanner *scanner
}

// Parse parses the string and returns a Tree.
func Parse(buf string) (*Tree, error) {
	t := new(Tree)
	t.scanner = new(scanner)
	return t.Parse(buf)
}

// Parse parses the string buffer to construct an ast
// representation for expansion.
func (t *Tree) Parse(buf string) (tree *Tree, err error) {
	t.scanner.init(buf)
	t.Root, err = t.parseAny()
	return t, err
}

func (t *Tree) parseAny() (Node, error) {
	t.scanner.accept = acceptRune
	t.scanner.mode = scanIdent | scanLbrack | scanEscape

	switch t.scanner.scan() {
	case tokenIdent:
		left := newTextNode(
			t.scanner.string(),
		)
		right, err := t.parseAny()
		switch {
		case err != nil:
			return nil, err
		case right == empty:
			return left, nil
		}
		return newListNode(left, right), nil
	case tokenEOF:
		return empty, nil
	case tokenLbrack:
		left, err := t.parseFunc()
		if err != nil {
			return nil, err
		}

		right, err := t.parseAny()
		switch {
		case err != nil:
			return nil, err
		case right == empty:
			return left, nil
		}
		return newListNode(left, right), nil
	}

	log.Println("Got a bad thing")
	return nil, ErrBadSubstitution
}

func (t *Tree) parseFunc() (Node, error) {
	switch t.scanner.peek() {
	case '#':
		return t.parseLenFunc()
	}

	var name string
	t.scanner.accept = acceptIdent
	t.scanner.mode = scanIdent

	switch tok := t.scanner.scan(); tok {
	case tokenIdent:
		name = t.scanner.string()
	default:
		return nil, &ErrParse{
			lineNumber: t.scanner.line,
			context:    t.scanner.context(),
			err:        fmt.Errorf("unable to parse variable name"),
		}
	}

	switch t.scanner.peek() {
	case ':':
		return t.parseDefaultOrSubstr(name)
	case '=':
		return t.parseDefaultFunc(name)
	case ',', '^':
		return t.parseCasingFunc(name)
	case '/':
		return t.parseReplaceFunc(name)
	case '#':
		return t.parseRemoveFunc(name, acceptHashFunc)
	case '%':
		return t.parseRemoveFunc(name, acceptPercentFunc)
	}

	t.scanner.accept = acceptIdent
	t.scanner.mode = scanRbrack
	switch t.scanner.scan() {
	case tokenRbrack:
		return newFuncNode(name), nil
	default:
		return nil, &ErrParse{
			lineNumber: t.scanner.line,
			context:    t.scanner.context(),
			err:        errors.New("missing closing brace"),
		}
	}
}

// parse a substitution function parameter.
func (t *Tree) parseParam(accept acceptFunc, mode byte) (Node, error) {
	t.scanner.accept = accept
	t.scanner.mode = mode | scanLbrack
	switch t.scanner.scan() {
	case tokenLbrack:
		return t.parseFunc()
	case tokenIdent:
		return newTextNode(
			t.scanner.string(),
		), nil
	case tokenRbrack:
		return newTextNode(
			t.scanner.string(),
		), nil
	default:
		return nil, errors.New("unable to parse substitution")
	}
}

// parse either a default or substring substitution function.
func (t *Tree) parseDefaultOrSubstr(name string) (Node, error) {
	// TODO: Do we need this additional read/unread
	t.scanner.read()
	r := t.scanner.peek()
	t.scanner.unread()
	switch r {
	case '=', '-', '?', '+':
		return t.parseDefaultFunc(name)
	default:
		return t.parseSubstrFunc(name)
	}
}

// parses the ${param:offset} string function
// parses the ${param:offset:length} string function
func (t *Tree) parseSubstrFunc(name string) (Node, error) {
	node := new(FuncNode)
	node.Param = name

	t.scanner.accept = acceptOneColon
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Name = t.scanner.string()
	default:
		return nil, ErrBadSubstitution
	}

	// scan arg[1]
	{
		param, err := t.parseParam(rejectColonClose, scanIdent)
		if err != nil {
			return nil, err
		}

		// param.Value = t.scanner.string()
		node.Args = append(node.Args, param)
	}

	// expect delimiter or close
	t.scanner.accept = acceptColon
	t.scanner.mode = scanIdent | scanRbrack
	switch t.scanner.scan() {
	case tokenRbrack:
		return node, nil
	case tokenIdent:
		// no-op
	default:
		return nil, ErrBadSubstitution
	}

	// scan arg[2]
	{
		param, err := t.parseParam(acceptNotClosing, scanIdent)
		if err != nil {
			return nil, err
		}
		node.Args = append(node.Args, param)
	}

	return node, t.consumeRbrack()
}

// parses the ${param%word} string function
// parses the ${param%%word} string function
// parses the ${param#word} string function
// parses the ${param##word} string function
func (t *Tree) parseRemoveFunc(name string, accept acceptFunc) (Node, error) {
	node := new(FuncNode)
	node.Param = name

	t.scanner.accept = accept
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Name = t.scanner.string()
	default:
		return nil, ErrBadSubstitution
	}

	// scan arg[1]
	{
		param, err := t.parseParam(acceptNotClosing, scanIdent)
		if err != nil {
			return nil, err
		}

		// param.Value = t.scanner.string()
		node.Args = append(node.Args, param)
	}

	return node, t.consumeRbrack()
}

// parses the ${param/pattern/string} string function
// parses the ${param//pattern/string} string function
// parses the ${param/#pattern/string} string function
// parses the ${param/%pattern/string} string function
func (t *Tree) parseReplaceFunc(name string) (Node, error) {
	node := new(FuncNode)
	node.Param = name

	t.scanner.accept = acceptReplaceFunc
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Name = t.scanner.string()
	default:
		return nil, ErrBadSubstitution
	}

	// scan arg[1]
	{
		param, err := t.parseParam(acceptNotSlash, scanIdent|scanEscape)
		if err != nil {
			return nil, err
		}
		node.Args = append(node.Args, param)
	}

	// expect delimiter
	t.scanner.accept = acceptSlash
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		// no-op
	default:
		return nil, ErrBadSubstitution
	}

	// check for blank string
	switch t.scanner.peek() {
	case '}':
		return node, t.consumeRbrack()
	}

	// scan arg[2]
	{
		param, err := t.parseParam(acceptNotClosing, scanIdent|scanEscape)
		if err != nil {
			return nil, err
		}
		node.Args = append(node.Args, param)
	}

	return node, t.consumeRbrack()
}

// parses the ${parameter=word} string function
// parses the ${parameter:=word} string function
// parses the ${parameter:-word} string function
// parses the ${parameter:?word} string function
// parses the ${parameter:+word} string function
func (t *Tree) parseDefaultFunc(name string) (Node, error) {
	node := new(FuncNode)
	node.Param = name

	println("---default func", name)
	t.scanner.accept = acceptDefaultFunc
	if t.scanner.peek() == '=' {
		t.scanner.accept = acceptOneEqual
	}
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Name = t.scanner.string()
		println("--ident found", node.Name, t.scanner.line)
	default:
		log.Printf("unable to parse default func, unexpected %s\n", t.scanner.context())
		return nil, ErrBadSubstitution
	}

	// loop through all possible runes in default param
	for {
		// this acts as the break condition. Peek to see if we reached the end
		switch t.scanner.peek() {
		case '}':
			return node, t.consumeRbrack()
		}
		param, err := t.parseParam(acceptNotClosing, scanIdent)
		if err != nil {
			return nil, &ErrParse{
				lineNumber: t.scanner.line,
				context:    t.scanner.context(),
				err:        err,
			}

		}

		node.Args = append(node.Args, param)
	}
}

// parses the ${param,} string function
// parses the ${param,,} string function
// parses the ${param^} string function
// parses the ${param^^} string function
func (t *Tree) parseCasingFunc(name string) (Node, error) {
	node := new(FuncNode)
	node.Param = name

	t.scanner.accept = acceptCasingFunc
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Name = t.scanner.string()
	default:
		return nil, ErrBadSubstitution
	}

	return node, t.consumeRbrack()
}

// parses the ${#param} string function
func (t *Tree) parseLenFunc() (Node, error) {
	node := new(FuncNode)

	t.scanner.accept = acceptOneHash
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Name = t.scanner.string()
	default:
		return nil, ErrBadSubstitution
	}

	t.scanner.accept = acceptIdent
	t.scanner.mode = scanIdent
	switch t.scanner.scan() {
	case tokenIdent:
		node.Param = t.scanner.string()
	default:
		return nil, ErrBadSubstitution
	}

	return node, t.consumeRbrack()
}

// consumeRbrack consumes a right closing bracket. If a closing
// bracket token is not consumed an ErrBadSubstitution is returned.
func (t *Tree) consumeRbrack() error {
	t.scanner.mode = scanRbrack
	if t.scanner.scan() != tokenRbrack {
		return ErrBadSubstitution
	}
	return nil
}

// consumeDelimiter consumes a function argument delimiter. If a
// delimiter is not consumed an ErrBadSubstitution is returned.
// func (t *Tree) consumeDelimiter(accept acceptFunc, mode uint) error {
// 	t.scanner.accept = accept
// 	t.scanner.mode = mode
// 	if t.scanner.scan() != tokenRbrack {
// 		return ErrBadSubstitution
// 	}
// 	return nil
// }
