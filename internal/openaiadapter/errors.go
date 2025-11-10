package openaiadapter

// ChatCompletionError represents an OpenAI-formatted error for chat completion endpoints.
// This is the standard error structure that OpenAI clients expect.
type ChatCompletionError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

// Error implements the error interface, returning the error message.
func (e *ChatCompletionError) Error() string {
	return e.Message
}

// ChatCompletionErrorResponse wraps ChatCompletionError in the SSE format
// that OpenAI streaming clients expect: {"error": {...}}
type ChatCompletionErrorResponse struct {
	// Err is the underlying error detail. JSON tag ensures it serializes as "error".
	Err *ChatCompletionError `json:"error"`
}

// Error implements the error interface, returning the underlying error message.
// This allows ChatCompletionErrorResponse to be used directly in error returns
// while maintaining the full OpenAI error structure for marshaling.
func (e *ChatCompletionErrorResponse) Error() string {
	if e.Err == nil {
		return "unknown error"
	}
	return e.Err.Message
}
