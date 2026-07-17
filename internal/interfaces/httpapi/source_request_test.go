/*
   Panvara
   internal/interfaces/httpapi/source_request_test.go    2026-07-16
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shezw/panvara/internal/domain/appmodule"
)

func TestReadDraftSourceAcceptsRawUTF8WithinBound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		contentType string
		source      []byte
		format      appmodule.SourceFormat
	}{
		{name: "empty json", contentType: "application/json", source: []byte{}, format: appmodule.SourceFormatJSON},
		{name: "invalid yaml is still a draft", contentType: "application/yaml; charset=utf-8", source: []byte("spec: ["), format: appmodule.SourceFormatYAML},
		{name: "exact boundary", contentType: "application/yaml", source: bytes.Repeat([]byte("x"), int(maxDraftSourceBytes)), format: appmodule.SourceFormatYAML},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/drafts", bytes.NewReader(test.source))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			got, format, problem := readDraftSource(response, request)
			if problem != nil || format != test.format || !bytes.Equal(got, test.source) {
				t.Fatalf("readDraftSource() = bytes:%d format:%q problem:%+v", len(got), format, problem)
			}
		})
	}
}

func TestReadDraftSourceRejectsUnsafeTransportAndEncoding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		contentType string
		encoding    string
		source      []byte
		wantStatus  int
		wantCode    string
	}{
		{name: "unknown type", contentType: "text/yaml", source: []byte("x"), wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_source_format"},
		{name: "non utf8 charset", contentType: "application/json; charset=iso-8859-1", source: []byte("{}"), wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_source_format"},
		{name: "blank charset", contentType: `application/json; charset=" "`, source: []byte("{}"), wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_source_format"},
		{name: "unknown media parameter", contentType: "application/json; profile=draft", source: []byte("{}"), wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_source_format"},
		{name: "content encoding", contentType: "application/json", encoding: "gzip", source: []byte("{}"), wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_content_encoding"},
		{name: "invalid utf8", contentType: "application/yaml", source: []byte{0xff}, wantStatus: http.StatusBadRequest, wantCode: "invalid_source_encoding"},
		{name: "nul byte", contentType: "application/yaml", source: []byte("name:\x00value"), wantStatus: http.StatusBadRequest, wantCode: "invalid_source_encoding"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/drafts", bytes.NewReader(test.source))
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("Content-Encoding", test.encoding)
			_, _, problem := readDraftSource(httptest.NewRecorder(), request)
			if problem == nil || problem.status != test.wantStatus || problem.code != test.wantCode {
				t.Fatalf("problem = %+v, want status/code %d/%s", problem, test.wantStatus, test.wantCode)
			}
		})
	}
}

func TestReadDraftSourceRejectsKnownAndChunkedOversizeBodies(t *testing.T) {
	t.Parallel()
	for _, knownLength := range []bool{true, false} {
		request := httptest.NewRequest(
			http.MethodPost, "/drafts", strings.NewReader(strings.Repeat("x", int(maxDraftSourceBytes)+1)),
		)
		request.Header.Set("Content-Type", "application/yaml")
		if !knownLength {
			request.ContentLength = -1
		}
		_, _, problem := readDraftSource(httptest.NewRecorder(), request)
		if problem == nil || problem.status != http.StatusRequestEntityTooLarge || problem.code != "source_too_large" {
			t.Fatalf("knownLength=%t problem=%+v", knownLength, problem)
		}
	}
}
