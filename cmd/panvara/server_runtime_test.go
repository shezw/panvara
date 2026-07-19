/*
   Panvara
   cmd/panvara/server_runtime_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appmodule "github.com/shezw/panvara/internal/application/appmodule"
)

func TestServerConfigAllowsEmptyBootstrapTokenForInitializedScope(t *testing.T) {
	t.Parallel()

	config := serverConfig{
		databaseURL:  "postgres://redacted",
		moduleSource: "crm.yaml",
		projectID:    "018f7e93-7b2c-7abc-8def-1234567890ab",
	}
	if err := config.validate(); err != nil {
		t.Fatalf("serverConfig.validate() with no restart token = %v", err)
	}
}

func TestReadModuleSourceIsBounded(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	smallPath := filepath.Join(directory, "small.yaml")
	if err := os.WriteFile(smallPath, []byte("kind: AppModule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := readModuleSource(smallPath)
	if err != nil || string(source) != "kind: AppModule\n" {
		t.Fatalf("readModuleSource(small) = %q, %v", source, err)
	}

	largePath := filepath.Join(directory, "large.yaml")
	if err := os.WriteFile(largePath, bytes.Repeat([]byte("x"), int(maxModuleSourceBytes)+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readModuleSource(largePath); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("readModuleSource(large) error = %v", err)
	}
}

func TestDocumentedExampleModuleCompiles(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "examples", "modules", "crm-leads.yaml")
	source, err := readModuleSource(path)
	if err != nil {
		t.Fatalf("read documented example: %v", err)
	}
	format, err := parseModuleFormat("auto", path)
	if err != nil {
		t.Fatalf("detect documented example format: %v", err)
	}
	module, err := appmodule.NewCompiler().Compile(source, format)
	if err != nil {
		t.Fatalf("compile documented example: %v", err)
	}
	if module.Name() != "crm.leads" || module.RevisionHash() == "" {
		t.Fatalf("documented example compiled as %q revision %q", module.Name(), module.RevisionHash())
	}
}

func TestReleasePublisherCompositionFailsClosedWithoutDependencies(t *testing.T) {
	t.Parallel()

	publisher, err := composeReleasePublisher(nil, nil, nil)
	if publisher != nil || err == nil || !strings.Contains(err.Error(), "release publisher") {
		t.Fatalf("composeReleasePublisher(nil) = %#v, %v", publisher, err)
	}
}
