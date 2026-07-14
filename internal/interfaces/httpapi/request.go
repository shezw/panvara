/*
   Panvara
   internal/interfaces/httpapi/request.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxJSONBodyBytes int64 = 256 << 10

type requestProblem struct {
	status  int
	code    string
	message string
}

func readJSONBody(writer http.ResponseWriter, request *http.Request) ([]byte, *requestProblem) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, &requestProblem{
			status: http.StatusUnsupportedMediaType, code: "unsupported_media_type",
			message: "Content-Type must be application/json",
		}
	}
	if request.ContentLength > maxJSONBodyBytes {
		return nil, &requestProblem{
			status: http.StatusRequestEntityTooLarge, code: "request_too_large",
			message: "request body exceeds 256 KiB",
		}
	}

	request.Body = http.MaxBytesReader(writer, request.Body, maxJSONBodyBytes)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, &requestProblem{
				status: http.StatusRequestEntityTooLarge, code: "request_too_large",
				message: "request body exceeds 256 KiB",
			}
		}
		return nil, &requestProblem{
			status: http.StatusBadRequest, code: "invalid_request_body",
			message: "could not read request body",
		}
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, &requestProblem{
			status: http.StatusBadRequest, code: "invalid_request_body",
			message: "request body must not be empty",
		}
	}
	return body, nil
}
