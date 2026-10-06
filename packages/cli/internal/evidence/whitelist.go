package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var sharedTokens = []string{"data.tokens.input", "data.tokens.output", "data.tokens.reasoning", "data.tokens.cache.read", "data.tokens.cache.write", "data.time.created", "data.time.completed"}

func paths(harness, format string) ([]string, bool) {
	switch harness {
	case "opencode":
		switch format {
		case "opencode-v1":
			return append([]string{"id", "session_id", "time_created", "data.role", "data.modelID", "data.providerID"}, sharedTokens...), true
		case "opencode-v2":
			return append([]string{"id", "session_id", "type", "time_created", "data.model.id", "data.model.providerID"}, sharedTokens...), true
		case "opencode-session":
			return []string{"id", "project_id"}, true
		case "opencode-project":
			return []string{"id", "vcs"}, true
		}
	case "pi":
		if format == "pi-jsonl" {
			return []string{"type", "id", "timestamp", "message.role", "message.timestamp", "message.provider", "message.model", "message.usage.input", "message.usage.output", "message.usage.reasoning", "message.usage.cacheRead", "message.usage.cacheWrite", "message.usage.totalTokens"}, true
		}
	case "claude-code":
		if format == "claude-code-jsonl" {
			return []string{"type", "timestamp", "uuid", "sessionId", "session_id", "requestId", "request_id", "message.role", "message.id", "message.provider", "message.provider_id", "message.providerID", "message.model", "message.model_id", "message.modelID", "message.usage.input_tokens", "message.usage.output_tokens", "message.usage.cache_read_input_tokens", "message.usage.cache_creation_input_tokens", "message.usage.total_tokens", "message.usage.output_tokens_details.thinking_tokens", "message.usage.output_tokens_details.reasoning_tokens"}, true
		}
	case "codex":
		if format != "codex-jsonl" {
			break
		}
		result := []string{"type", "timestamp", "payload.type", "payload.id", "payload.model_provider", "payload.forked_from_id", "payload.parent_thread_id", "payload.source.subagent.thread_spawn.parent_thread_id", "payload.turn_id", "payload.model"}
		for _, prefix := range []string{"payload.info.last_token_usage.", "payload.info.total_token_usage."} {
			for _, leaf := range []string{"input_tokens", "cached_input_tokens", "cache_read_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens"} {
				result = append(result, prefix+leaf)
			}
		}
		return result, true
	}
	return nil, false
}

func decodeObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return nil, errors.New("invalid_source_record")
	}
	if object == nil || decoder.Decode(new(interface{})) != io.EOF {
		return nil, errors.New("invalid_source_record")
	}
	return object, nil
}

func leaf(object map[string]json.RawMessage, parts []string) (json.RawMessage, bool) {
	value, found := object[parts[0]]
	if !found || len(parts) == 1 {
		return value, found
	}
	child, err := decodeObject(value)
	if err != nil {
		return nil, false
	}
	return leaf(child, parts[1:])
}

func setLeaf(object map[string]interface{}, parts []string, value json.RawMessage) {
	if len(parts) == 1 {
		object[parts[0]] = value
		return
	}
	child, ok := object[parts[0]].(map[string]interface{})
	if !ok {
		child = map[string]interface{}{}
		object[parts[0]] = child
	}
	setLeaf(child, parts[1:], value)
}

func SafeMetadata(value string) bool {
	if !utf8.ValidString(value) || len(value) > MaxStringBytes || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") || strings.Contains(value, "://") || strings.Contains(value, "@") || strings.Contains(value, "\\") {
		return false
	}
	if len(value) >= 3 && value[1] == ':' && value[2] == '/' {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Sanitize copies only approved scalar leaves. Unknown objects are never copied.
func Sanitize(harness, format string, body []byte) (json.RawMessage, []string, error) {
	allowed, ok := paths(harness, format)
	if !ok {
		return nil, nil, errors.New("unsupported_source_format")
	}
	object, err := decodeObject(body)
	if err != nil {
		return nil, nil, err
	}
	result := map[string]interface{}{}
	diagnosticSet := map[string]bool{}
	for _, path := range allowed {
		parts := strings.Split(path, ".")
		value, found := leaf(object, parts)
		if !found {
			continue
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || trimmed[0] == '{' || trimmed[0] == '[' {
			diagnosticSet["invalid_field_type"] = true
			setLeaf(result, parts, json.RawMessage("null"))
			continue
		}
		if trimmed[0] == '"' {
			var text string
			if json.Unmarshal(trimmed, &text) != nil || !SafeMetadata(text) {
				diagnosticSet["unsafe_metadata"] = true
				setLeaf(result, parts, json.RawMessage("null"))
				continue
			}
		}
		if len(value) > MaxStringBytes {
			diagnosticSet["field_limit"] = true
			setLeaf(result, parts, json.RawMessage("null"))
			continue
		}
		setLeaf(result, parts, value)
	}
	if harness == "codex" {
		if value, found := leaf(object, []string{"payload", "source", "subagent"}); found && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			diagnosticSet["native_subagent_present"] = true
		}
	}
	var diagnostics []string
	for code := range diagnosticSet {
		diagnostics = append(diagnostics, code)
	}
	sort.Strings(diagnostics)
	out, err := json.Marshal(result)
	return out, diagnostics, err
}

func DecodeData(body []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var record map[string]interface{}
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if record == nil || decoder.Decode(new(interface{})) != io.EOF {
		return nil, errors.New("invalid_record")
	}
	return record, nil
}

func String(body json.RawMessage, path string) string {
	object, err := decodeObject(body)
	if err != nil {
		return ""
	}
	value, ok := leaf(object, strings.Split(path, "."))
	if !ok {
		return ""
	}
	var text string
	_ = json.Unmarshal(value, &text)
	return text
}

func UsageRecord(harness, format string, data json.RawMessage) bool {
	switch harness {
	case "opencode":
		return format == "opencode-v1" && String(data, "data.role") == "assistant" || format == "opencode-v2" && String(data, "type") == "assistant"
	case "pi":
		return String(data, "type") == "message" && String(data, "message.role") == "assistant"
	case "claude-code":
		return String(data, "type") == "assistant" && String(data, "message.role") == "assistant"
	case "codex":
		return String(data, "type") == "event_msg" && String(data, "payload.type") == "token_count"
	}
	return false
}

// Scope uses native context for scheduling only; it is not a contribution ID.
func Scope(record Record) string {
	native := ""
	switch record.Harness {
	case "opencode":
		native = String(record.Data, "session_id")
		if record.Format == "opencode-session" {
			native = String(record.Data, "id")
		}
	case "claude-code":
		native = String(record.Data, "sessionId")
		if native == "" {
			native = String(record.Data, "session_id")
		}
	case "pi":
		if String(record.Data, "type") == "session" {
			native = String(record.Data, "id")
		}
	case "codex":
		if String(record.Data, "type") == "session_meta" {
			native = String(record.Data, "payload.id")
		}
	}
	if native == "" {
		for _, context := range record.Context {
			if record.Harness == "pi" && String(context.Data, "type") == "session" {
				native = String(context.Data, "id")
			}
			if record.Harness == "codex" && String(context.Data, "type") == "session_meta" {
				native = String(context.Data, "payload.id")
			}
		}
	}
	if native == "" {
		return record.Harness + ":source:" + Tuple(record.SourceID, record.Lineage)
	}
	return SessionScope(record.Harness, native)
}

func SessionScope(harness, nativeID string) string { return harness + ":session:" + nativeID }

func NativeRecordKey(record Record) string {
	scope := Scope(record)
	if strings.HasPrefix(scope, record.Harness+":source:") {
		return ""
	}
	id := ""
	switch record.Harness {
	case "opencode":
		id = String(record.Data, "id")
	case "pi":
		id = String(record.Data, "id")
	case "claude-code":
		id = String(record.Data, "uuid")
		if id == "" {
			id = String(record.Data, "message.id") + ":" + String(record.Data, "requestId") + ":" + String(record.Data, "request_id")
		}
	}
	if id == "" || id == "::" {
		return ""
	}
	return Tuple(record.Harness, record.Format, scope, id)
}

func EvidenceID(record Record) string {
	key := NativeRecordKey(record)
	if key == "" {
		key = Tuple(record.Harness, record.Format, record.SourceID, record.Lineage, record.Ordinal)
	}
	return Tuple(key, record.Data, record.Context, record.Location, record.Diagnostics)
}
