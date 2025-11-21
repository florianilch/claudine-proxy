package types

// Error implements the error interface for Error, returning the error message.
func (e *Error) Error() string {
	return e.Message
}

// Error implements the error interface for ErrorResponse, returning the underlying error message.
// This allows ErrorResponse to be used directly in error returns.
func (e *ErrorResponse) Error() string {
	return e.Err.Message
}

// Error implements the error interface for ErrorEvent, returning the underlying error message.
// This allows ErrorEvent to be used in SSE streaming error responses.
func (e *ErrorEvent) Error() string {
	return e.Data.Message
}
