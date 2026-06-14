package extension

import (
	"encoding/json"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestResolveSettingsDefaultsOnly(t *testing.T) {
	schema := json.RawMessage(`{
		"useTorrent": {"label": "Use torrent", "type": "boolean", "value": false},
		"limit": {"label": "Limit", "type": "number", "default": 25}
	}`)
	got := resolveSettings(schema, nil)
	if got["useTorrent"] != false {
		t.Errorf("useTorrent = %v, want false (from value)", got["useTorrent"])
	}
	// JSON numbers decode to float64.
	if got["limit"] != float64(25) {
		t.Errorf("limit = %v, want 25 (from default)", got["limit"])
	}
}

func TestResolveSettingsDBOverridesDefault(t *testing.T) {
	schema := json.RawMessage(`{"useTorrent": {"value": false}}`)
	stored := strPtr(`{"useTorrent": true}`)
	got := resolveSettings(schema, stored)
	if got["useTorrent"] != true {
		t.Errorf("useTorrent = %v, want true (DB override wins)", got["useTorrent"])
	}
}

func TestResolveSettingsUnknownDBKeysPreserved(t *testing.T) {
	schema := json.RawMessage(`{"a": {"value": 1}}`)
	stored := strPtr(`{"b": "extra"}`)
	got := resolveSettings(schema, stored)
	if got["a"] != float64(1) {
		t.Errorf("a = %v, want 1 (default kept)", got["a"])
	}
	if got["b"] != "extra" {
		t.Errorf("b = %v, want extra (unknown DB key preserved)", got["b"])
	}
}

func TestResolveSettingsNilNilEmpty(t *testing.T) {
	got := resolveSettings(nil, nil)
	if got == nil {
		t.Fatal("resolveSettings(nil,nil) returned nil, want empty non-nil map")
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %v", got)
	}
}

func TestResolveSettingsMalformedStoredKeepsDefaults(t *testing.T) {
	schema := json.RawMessage(`{"a": {"value": 1}}`)
	stored := strPtr(`{not valid json`)
	got := resolveSettings(schema, stored)
	if got["a"] != float64(1) {
		t.Errorf("malformed stored should keep defaults; a = %v", got["a"])
	}
}

func TestResolveSettingsBareScalarEntries(t *testing.T) {
	// All three shapes in one schema: {value}, {default}, and a bare scalar.
	schema := json.RawMessage(`{
		"withValue": {"value": "v"},
		"withDefault": {"default": "d"},
		"bare": "scalar"
	}`)
	got := resolveSettings(schema, nil)
	if got["withValue"] != "v" {
		t.Errorf("withValue = %v, want v", got["withValue"])
	}
	if got["withDefault"] != "d" {
		t.Errorf("withDefault = %v, want d", got["withDefault"])
	}
	if got["bare"] != "scalar" {
		t.Errorf("bare = %v, want scalar", got["bare"])
	}
}

func TestResolveSettingsValuePreferredOverDefault(t *testing.T) {
	schema := json.RawMessage(`{"k": {"value": "fromValue", "default": "fromDefault"}}`)
	got := resolveSettings(schema, nil)
	if got["k"] != "fromValue" {
		t.Errorf("k = %v, want fromValue (value preferred over default)", got["k"])
	}
}

func TestResolveSettingsMalformedSchemaIgnored(t *testing.T) {
	got := resolveSettings(json.RawMessage(`not json`), strPtr(`{"x": 1}`))
	if got["x"] != float64(1) {
		t.Errorf("malformed schema should be ignored, stored kept; x = %v", got["x"])
	}
}
