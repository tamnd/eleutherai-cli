package cli

import "fmt"

// exitCoder carries a process exit code alongside an error message.
type exitCoder struct {
	err  error
	code int
}

func (e exitCoder) Error() string { return e.err.Error() }
func (e exitCoder) ExitCode() int { return e.code }

func noResults(msg string) error { return exitCoder{fmt.Errorf("%s", msg), 3} }
