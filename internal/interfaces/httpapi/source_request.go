/*
   Panvara
   internal/interfaces/httpapi/source_request.go    2026-07-16
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
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/shezw/panvara/internal/domain/appmodule"
)

const maxDraftSourceBytes int64 = 1 << 20

func readDraftSource(
	writer http.ResponseWriter,
	request *http.Request,
) ([]byte, appmodule.SourceFormat, *requestProblem) {
	if strings.TrimSpace(request.Header.Get("Content-Encoding")) != "" {
		return nil, "", &requestProblem{
			status: http.StatusUnsupportedMediaType, code: "unsupported_content_encoding",
			message: "Content-Encoding is not supported for AppModule source",
		}
	}
	mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", unsupportedDraftSourceType()
	}
	charsetValue, hasCharset := parameters["charset"]
	if len(parameters) != 0 && (!hasCharset || len(parameters) != 1) {
		return nil, "", unsupportedDraftSourceType()
	}
	charset := strings.ToLower(strings.TrimSpace(charsetValue))
	if (hasCharset && charset == "") || (charset != "" && charset != "utf-8" && charset != "utf8") {
		return nil, "", unsupportedDraftSourceType()
	}
	var format appmodule.SourceFormat
	switch strings.ToLower(mediaType) {
	case "application/json":
		format = appmodule.SourceFormatJSON
	case "application/yaml":
		format = appmodule.SourceFormatYAML
	default:
		return nil, "", unsupportedDraftSourceType()
	}
	if request.ContentLength > maxDraftSourceBytes {
		return nil, "", draftSourceTooLarge()
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxDraftSourceBytes)
	source, err := io.ReadAll(request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, "", draftSourceTooLarge()
		}
		return nil, "", &requestProblem{
			status: http.StatusBadRequest, code: "invalid_request_body",
			message: "could not read AppModule source",
		}
	}
	if !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return nil, "", &requestProblem{
			status: http.StatusBadRequest, code: "invalid_source_encoding",
			message: "AppModule source must be NUL-free valid UTF-8 text",
		}
	}
	return source, format, nil
}

func unsupportedDraftSourceType() *requestProblem {
	return &requestProblem{
		status: http.StatusUnsupportedMediaType, code: "unsupported_source_format",
		message: "Content-Type must be application/json or application/yaml with UTF-8 encoding",
	}
}

func draftSourceTooLarge() *requestProblem {
	return &requestProblem{
		status: http.StatusRequestEntityTooLarge, code: "source_too_large",
		message: "AppModule source exceeds 1 MiB",
	}
}
