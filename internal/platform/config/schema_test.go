// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaJSON_ValidJSON(t *testing.T) {
	raw, err := SchemaJSON()
	if err != nil {
		t.Fatalf("SchemaJSON: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, raw)
	}
	if schema["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Errorf("$schema header missing/wrong: %v", schema["$schema"])
	}
	if _, ok := schema["properties"]; !ok {
		t.Error("missing top-level 'properties'")
	}
}

func TestSchemaJSON_HasOutputProperty(t *testing.T) {
	raw, _ := SchemaJSON()
	var schema map[string]any
	_ = json.Unmarshal(raw, &schema)
	props := schema["properties"].(map[string]any)
	if _, ok := props["output"]; !ok {
		t.Error("schema missing 'output' property")
	}
}

func TestSchemaJSON_OneofBecomesEnum(t *testing.T) {
	raw, _ := SchemaJSON()
	if !strings.Contains(string(raw), `"enum"`) {
		t.Error("schema should contain enum (from validate:oneof)")
	}
	if !strings.Contains(string(raw), `"text"`) {
		t.Error("schema should contain 'text' enum value (from Output.Format oneof)")
	}
}

func TestSchemaJSON_ExposesEnvAndRedactExtensions(t *testing.T) {
	raw, _ := SchemaJSON()
	out := string(raw)
	if !strings.Contains(out, `"x-env"`) {
		t.Error("schema missing x-env extension")
	}
	if !strings.Contains(out, `"x-redact"`) {
		t.Error("schema missing x-redact extension")
	}
}

func TestWriteSchemaJSON_TrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSchemaJSON(&buf); err != nil {
		t.Fatalf("WriteSchemaJSON: %v", err)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("WriteSchemaJSON should end with newline for tooling compatibility")
	}
}

func TestWriteEnvMap_SortedAndContainsAllSecrets(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEnvMap(&buf); err != nil {
		t.Fatalf("WriteEnvMap: %v", err)
	}
	out := buf.String()
	for _, env := range []string{"OPENDBX_LLM_API_KEY", "OPENDBX_OUTPUT_FORMAT", "OPENDBX_SECURITY_DEFAULT_LEVEL"} {
		if !strings.Contains(out, env) {
			t.Errorf("WriteEnvMap missing %s", env)
		}
	}
	// Sorted check: OPENDBX_LLM_* lines should appear before OPENDBX_OUTPUT_*.
	lLLM := strings.Index(out, "OPENDBX_LLM_API_KEY")
	lOutput := strings.Index(out, "OPENDBX_OUTPUT_FORMAT")
	if lLLM >= lOutput {
		t.Error("WriteEnvMap output not sorted alphabetically")
	}
}

func TestWriteSources_AllTopLevel(t *testing.T) {
	cfg := Default()
	var buf bytes.Buffer
	if err := WriteSources(&buf, cfg, ""); err != nil {
		t.Fatalf("WriteSources: %v", err)
	}
	out := buf.String()
	for _, name := range []string{"Security", "Output", "LLM", "Session", "Sentinel", "Trace", "Scheduler"} {
		if !strings.Contains(out, name) {
			t.Errorf("WriteSources missing %s", name)
		}
		if !strings.Contains(out, "default") {
			t.Errorf("WriteSources should mark fresh fields as 'default'")
		}
	}
}

func TestWriteSources_SingleField(t *testing.T) {
	cfg := Default()
	cfg.SetSource("LLM", SourceENV)
	var buf bytes.Buffer
	if err := WriteSources(&buf, cfg, "LLM.RequestTimeout"); err != nil {
		t.Fatalf("WriteSources: %v", err)
	}
	if !strings.Contains(buf.String(), "env") {
		t.Errorf("WriteSources should report 'env' source: got %q", buf.String())
	}
}

func TestValidateFile_OK(t *testing.T) {
	tmp := t.TempDir() + "/cfg.yaml"
	if err := writeFile(tmp, "output:\n  format: json\n"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(tmp); err != nil {
		t.Errorf("validate OK file failed: %v", err)
	}
}

func TestValidateFile_MissingFails(t *testing.T) {
	if err := ValidateFile("/nonexistent/foo.yaml"); err == nil {
		t.Error("expected error on missing file")
	}
}

func TestValidateFile_TooLarge(t *testing.T) {
	tmp := t.TempDir() + "/big.yaml"
	huge := strings.Repeat("a", 1<<20+1)
	if err := writeFile(tmp, huge); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(tmp); err == nil {
		t.Error("expected error on >1MB file")
	}
}
