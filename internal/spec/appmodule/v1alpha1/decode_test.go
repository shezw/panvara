/*
   Panvara
   internal/spec/appmodule/v1alpha1/decode_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package v1alpha1

import (
	"errors"
	"strings"
	"testing"
)

const validYAMLDocument = `apiVersion: panvara.dev/v1alpha1
kind: AppModule
metadata:
  name: crm.leads
  version: 1.2.3
  labels:
    en-US: CRM Leads
spec:
  resources: []
`

const validJSONDocument = `{"apiVersion":"panvara.dev/v1alpha1","kind":"AppModule","metadata":{"name":"crm.leads","version":"1.2.3"},"spec":{"resources":[]}}`

func TestDecodeAcceptsExplicitStrictFormats(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		source string
		format Format
	}{
		{name: "json", source: validJSONDocument, format: FormatJSON},
		{name: "yaml", source: validYAMLDocument, format: FormatYAML},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document, err := Decode([]byte(test.source), test.format)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if document.Metadata.Name != "crm.leads" || document.Metadata.Version != "1.2.3" {
				t.Fatalf("Decode() metadata = %#v", document.Metadata)
			}
		})
	}
}

func TestDecodeJSONRejectsHiddenOrAmbiguousInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "unknown exact-case field", source: `{"apiVersion":"panvara.dev/v1alpha1","Kind":"AppModule","metadata":{},"spec":{}}`},
		{name: "duplicate key", source: `{"apiVersion":"panvara.dev/v1alpha1","kind":"AppModule","kind":"AppModule","metadata":{},"spec":{}}`},
		{name: "trailing value", source: validJSONDocument + ` {}`},
		{name: "root array", source: `[]`},
		{name: "null field", source: `{"apiVersion":"panvara.dev/v1alpha1","kind":"AppModule","metadata":{"name":"crm","version":"1.0.0"},"spec":null}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode([]byte(test.source), FormatJSON); err == nil {
				t.Fatal("Decode() unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeYAMLRejectsUnsafeFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "unknown field", source: validYAMLDocument + "unknown: true\n"},
		{name: "multiple documents", source: validYAMLDocument + "---\n{}\n"},
		{name: "anchor", source: strings.Replace(validYAMLDocument, "metadata:\n", "metadata: &metadata\n", 1)},
		{name: "custom tag", source: strings.Replace(validYAMLDocument, "metadata:\n", "metadata: !metadata\n", 1)},
		{name: "null", source: strings.Replace(validYAMLDocument, "spec:\n", "spec: null\nignored:\n", 1)},
		{name: "merge", source: validYAMLDocument + "extra: &extra {kind: AppModule}\nmerged:\n  <<: *extra\n"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode([]byte(test.source), FormatYAML); err == nil {
				t.Fatal("Decode() unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeRejectsImplicitFormatInvalidUTF8AndOversize(t *testing.T) {
	t.Parallel()

	if _, err := Decode([]byte(validJSONDocument), ""); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Decode() empty format error = %v, want ErrUnsupportedFormat", err)
	}
	if _, err := Decode([]byte{0xff}, FormatYAML); err == nil {
		t.Fatal("Decode() accepted invalid UTF-8")
	}
	if _, err := Decode(make([]byte, defaultMaxDocumentBytes+1), FormatYAML); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("Decode() oversized error = %v, want ErrDocumentTooLarge", err)
	}
}
