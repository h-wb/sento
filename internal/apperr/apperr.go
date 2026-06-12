// Package apperr classifies errors at the service boundary so the HTTP
// layer can map them to status codes in exactly one place. Services tag
// errors with the constructors; handlers match kinds with errors.Is and
// never inspect error strings.
package apperr

import (
	"errors"
	"fmt"
)

// Kind sentinels. Match with errors.Is; create tagged errors with the
// constructors below.
var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid")     // bad input: a 400
	ErrConflict    = errors.New("conflict")    // state refuses the change: a 409
	ErrUpstream    = errors.New("upstream")    // a dependency failed: a 502
	ErrUnavailable = errors.New("unavailable") // a dependency is not configured: a 503
)

// kindError carries a clean message while matching its kind sentinel
// through errors.Is and keeping any %w-wrapped cause unwrappable.
type kindError struct {
	kind error
	msg  string
	err  error
}

func (e *kindError) Error() string        { return e.msg }
func (e *kindError) Is(target error) bool { return target == e.kind }
func (e *kindError) Unwrap() error        { return e.err }

func newf(kind error, format string, a ...any) error {
	err := fmt.Errorf(format, a...)
	return &kindError{kind: kind, msg: err.Error(), err: errors.Unwrap(err)}
}

func NotFoundf(format string, a ...any) error    { return newf(ErrNotFound, format, a...) }
func Invalidf(format string, a ...any) error     { return newf(ErrInvalid, format, a...) }
func Conflictf(format string, a ...any) error    { return newf(ErrConflict, format, a...) }
func Upstreamf(format string, a ...any) error    { return newf(ErrUpstream, format, a...) }
func Unavailablef(format string, a ...any) error { return newf(ErrUnavailable, format, a...) }
