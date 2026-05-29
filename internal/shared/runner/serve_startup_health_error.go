package runner

type serveStartupHealthError struct {
	err error
}

func (e *serveStartupHealthError) Error() string {
	return e.err.Error()
}

func (e *serveStartupHealthError) Unwrap() error {
	return e.err
}
