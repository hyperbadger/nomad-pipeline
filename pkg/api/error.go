package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Error struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Details string `json:"details"`
}

const (
	ErrorTypeNomadUpstream = "nomad_upstream"
	ErrorTypeNotFound      = "not_found"
	ErrorBadRequest        = "bad_request"
)

type ErrorOption func(*Error)

func NewError(opts ...ErrorOption) *Error {
	err := &Error{
		Code:   http.StatusInternalServerError,
		Status: http.StatusText(http.StatusInternalServerError),
	}

	for _, opt := range opts {
		opt(err)
	}

	return err
}

func (err *Error) Apply(c *gin.Context, logger *zap.SugaredLogger) {
	c.JSON(err.Code, gin.H{"error": err})

	logger.Errorw(
		err.Message,
		"code", err.Code,
		"status", err.Status,
		"type", err.Type,
		"details", err.Details,
	)
}

func WithCode(code int) ErrorOption {
	return func(err *Error) {
		err.Code = code
		err.Status = http.StatusText(code)
	}
}

func WithType(errType string) ErrorOption {
	return func(err *Error) {
		err.Type = errType
	}
}

func WithMessage(msg string) ErrorOption {
	return func(err *Error) {
		err.Message = msg
	}
}

func WithError(err error) ErrorOption {
	return func(_err *Error) {
		_err.Details = err.Error()
	}
}
