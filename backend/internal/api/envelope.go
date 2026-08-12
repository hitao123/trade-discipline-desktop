package api

// FieldError maps JSON field names to user-facing Chinese messages.
type FieldError map[string]string

type ErrorBody struct {
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Fields  FieldError `json:"fields,omitempty"`
}

type Envelope[T any] struct {
	OK    bool       `json:"ok"`
	Data  *T         `json:"data,omitempty"`
	Error *ErrorBody `json:"error,omitempty"`
}

func Success[T any](data T) Envelope[T] {
	return Envelope[T]{OK: true, Data: &data}
}

func Failure(code, message string, fields FieldError) Envelope[any] {
	return Envelope[any]{
		OK: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
			Fields:  fields,
		},
	}
}
