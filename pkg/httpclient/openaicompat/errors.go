package openaicompat

import (
	"fmt"
	"strings"
)

type ErrorKind string

const (
	ErrorKindRequest   ErrorKind = "request"
	ErrorKindTransport ErrorKind = "transport"
	ErrorKindHTTP      ErrorKind = "http"
	ErrorKindAPI       ErrorKind = "api"
	ErrorKindDecode    ErrorKind = "decode"
	ErrorKindAuth      ErrorKind = "auth"
)

type APIError struct {
	Kind       ErrorKind `json:"kind"`
	Operation  string    `json:"operation,omitempty"`
	Method     string    `json:"method,omitempty"`
	URL        string    `json:"url,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	Message    string    `json:"message,omitempty"`
	Type       string    `json:"type,omitempty"`
	Param      string    `json:"param,omitempty"`
	Code       any       `json:"code,omitempty"`
	Body       string    `json:"body,omitempty"`
	Retryable  bool      `json:"retryable,omitempty"`
	Cause      error     `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = strings.TrimSpace(e.Body)
	}
	switch e.Kind {
	case ErrorKindAPI:
		if e.StatusCode > 0 {
			return fmt.Sprintf("api error %d: %s", e.StatusCode, message)
		}
		return "api error: " + message
	case ErrorKindHTTP:
		if e.StatusCode > 0 {
			return fmt.Sprintf("http error %d: %s", e.StatusCode, message)
		}
		return "http error: " + message
	case ErrorKindTransport:
		if e.Cause != nil {
			return "http request failed: " + e.Cause.Error()
		}
		return "http request failed"
	case ErrorKindDecode:
		if e.Cause != nil {
			return "failed to decode response: " + e.Cause.Error()
		}
		return "failed to decode response"
	case ErrorKindRequest:
		if e.Cause != nil {
			return "failed to create request: " + e.Cause.Error()
		}
		return "failed to create request"
	case ErrorKindAuth:
		if e.Cause != nil {
			return "auth failed: " + e.Cause.Error()
		}
		return "auth failed: " + message
	default:
		if e.Cause != nil && message != "" {
			return message + ": " + e.Cause.Error()
		}
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return message
	}
}

func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func retryableStatus(status int) bool {
	return status == 408 || status == 409 || status == 425 || status == 429 || status >= 500
}
