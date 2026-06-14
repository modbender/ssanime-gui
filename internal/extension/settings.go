package extension

import "encoding/json"

// resolveSettings computes the flat per-extension settings map passed to a JS
// extension as the second method argument. It starts from the index.json
// `options` schema's per-key default value, then overlays the stored DB
// settings (a flat {key:value} JSON object) so user edits win. Both inputs are
// optional: a nil/empty/malformed schema or stored blob is tolerated and simply
// contributes nothing.
//
// The Hayase options schema shapes each entry one of three ways:
//
//	"useTorrent": { "label": "...", "type": "boolean", "value": false }
//	"useTorrent": { "label": "...", "type": "boolean", "default": false }
//	"useTorrent": false   // a bare scalar
//
// resolveSettings extracts the effective default for each: "value" then
// "default" for object entries, or the scalar itself for bare entries.
func resolveSettings(optionsSchema json.RawMessage, storedJSON *string) map[string]interface{} {
	out := map[string]interface{}{}

	// 1. Defaults from the schema.
	if len(optionsSchema) > 0 {
		var schema map[string]json.RawMessage
		if err := json.Unmarshal(optionsSchema, &schema); err == nil {
			for key, rawEntry := range schema {
				if v, ok := schemaDefault(rawEntry); ok {
					out[key] = v
				}
			}
		}
	}

	// 2. Overlay stored DB settings (flat key:value). DB wins; unknown DB keys
	//    are preserved.
	if storedJSON != nil && *storedJSON != "" {
		var stored map[string]interface{}
		if err := json.Unmarshal([]byte(*storedJSON), &stored); err == nil {
			for k, v := range stored {
				out[k] = v
			}
		}
	}

	return out
}

// schemaDefault extracts the effective default for one options-schema entry.
// For an object entry it prefers "value" then "default"; for a bare scalar it
// returns the scalar. ok is false when the entry yields no usable default (e.g.
// an object with neither key, or a null).
func schemaDefault(raw json.RawMessage) (interface{}, bool) {
	// Object entry: look for value/default.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil && obj != nil {
		for _, key := range []string{"value", "default"} {
			if v, ok := obj[key]; ok {
				var decoded interface{}
				if json.Unmarshal(v, &decoded) == nil {
					return decoded, true
				}
			}
		}
		return nil, false
	}
	// Bare scalar (string/number/bool). null yields no default.
	var scalar interface{}
	if err := json.Unmarshal(raw, &scalar); err == nil && scalar != nil {
		return scalar, true
	}
	return nil, false
}
