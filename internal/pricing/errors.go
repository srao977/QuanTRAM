package pricing

// This file centralizes formatted package errors.

import "fmt"

func errf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
