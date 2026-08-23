package service

import "errors"

type CodedError struct {
	Code    string
	Message string
}

func (e CodedError) Error() string { return e.Message }

func ErrorCode(err error) (string, bool) {
	var coded CodedError
	if !errors.As(err, &coded) {
		return "", false
	}
	return coded.Code, true
}
