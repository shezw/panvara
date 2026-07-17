/*
   Panvara
   internal/spec/appmodule/v1alpha1/schema_test.go    2026-07-14
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
	"encoding/json"
	"testing"
)

func TestAuthoringSchemaIsValidJSONAndMatchesEnvelopeConstants(t *testing.T) {
	t.Parallel()

	var schema map[string]any
	if err := json.Unmarshal(AuthoringSchema(), &schema); err != nil {
		t.Fatalf("AuthoringSchema() is not valid JSON: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties are missing")
	}
	apiVersion := properties["apiVersion"].(map[string]any)["const"]
	kind := properties["kind"].(map[string]any)["const"]
	if apiVersion != APIVersion || kind != Kind {
		t.Fatalf("schema envelope = %v/%v, want %s/%s", apiVersion, kind, APIVersion, Kind)
	}
	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		t.Fatal("root schema must reject additional properties")
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema definitions are missing")
	}
	labels, ok := definitions["labels"].(map[string]any)
	if !ok {
		t.Fatal("labels definition is missing")
	}
	labelValues, ok := labels["additionalProperties"].(map[string]any)
	if !ok || labelValues["pattern"] != `^[^\u0000]*$` {
		t.Fatalf("label value NUL exclusion = %#v", labelValues["pattern"])
	}
	first := AuthoringSchema()
	first[0] = '!'
	if AuthoringSchema()[0] == '!' {
		t.Fatal("AuthoringSchema() leaked embedded mutable bytes")
	}
}
