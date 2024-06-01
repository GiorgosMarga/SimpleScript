package interpreter

import "errors"

var (
	ErrSignalKilled       = errors.New("termination signal")
	ErrUndeclaredVariable = errors.New("undeclared variable")
	ErrInvalidOperation   = errors.New("invalid operation")
	ErrInvalidInstruction = errors.New("invalid instruction")
	ErrInvalidVarName     = errors.New("invalid variable name")
	ErrInvalidVal         = errors.New("invalid variable value")
	ErrInvalidLabel       = errors.New("invalid label value")
	ErrInvalidIntVal      = errors.New("invalid int value")
	ErrInvalidStringVal   = errors.New("invalid string value")
)
