package application

import "fmt"

// InputError identifies a predictable, user-correctable input failure without
// coupling application services to a presentation language.
type InputError struct {
	Code string
	Args []any
}

func (e InputError) Error() string {
	if len(e.Args) == 0 {
		return e.Code
	}
	return fmt.Sprintf("%s %v", e.Code, e.Args)
}

func NewInputError(code string, args ...any) error {
	return InputError{Code: code, Args: args}
}
