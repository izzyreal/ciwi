package server

import (
	"net/http"

	"github.com/izzyreal/ciwi/internal/application"
)

func applicationErrorHTTPStatus(err error) int {
	switch application.ErrorKindOf(err) {
	case application.ErrorInvalidArgument:
		return http.StatusBadRequest
	case application.ErrorNotFound:
		return http.StatusNotFound
	case application.ErrorConflict:
		return http.StatusConflict
	case application.ErrorFailedPrecondition:
		return http.StatusPreconditionFailed
	case application.ErrorUnavailable:
		return http.StatusServiceUnavailable
	case application.ErrorUnsupported:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}
