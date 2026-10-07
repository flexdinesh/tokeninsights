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

// sourceObject decodes each selected object at most once. RawMessage keeps
// native numbers exact and leaves private scalar/object values uninterpreted.
// A nil cached child remembers an invalid object shape as well.
type sourceObject struct {
	fields   map[string]json.RawMessage
	children map[string]*sourceObject
}

func parseSourceObject(body []byte) (*sourceObject, error) {
	fields, err := decodeObject(body)
	if err != nil {
		return nil, err
	}
	return &sourceObject{fields: fields}, nil
}

func (object *sourceObject) leaf(parts []string) (json.RawMessage, bool) {
	if object == nil {
		return nil, false
	}
	value, found := object.fields[parts[0]]
	if !found || len(parts) == 1 {
		return value, found
	}
	child, cached := object.children[parts[0]]
	if !cached {
		child, _ = parseSourceObject(value)
		if object.children == nil {
			object.children = make(map[string]*sourceObject)
		}
		object.children[parts[0]] = child
	}
	return child.leaf(parts[1:])
}

func (object *sourceObject) string(path string) string {
	value, ok := object.leaf(strings.Split(path, "."))
	if !ok {
		return ""
	}
	var text string
	_ = json.Unmarshal(value, &text)
	return text
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
	object, err := parseSourceObject(body)
	if err != nil {
		return nil, nil, err
	}
	result := map[string]interface{}{}
	diagnosticSet := map[string]bool{}
	for _, path := range allowed {
		parts := strings.Split(path, ".")
		value, found := object.leaf(parts)
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
		// Metadata bounds apply to decoded UTF-8, already checked above. JSON
		// quotes/escapes must not shorten the permitted native identifier.
		if trimmed[0] != '"' && len(value) > MaxStringBytes {
			diagnosticSet["field_limit"] = true
			setLeaf(result, parts, json.RawMessage("null"))
			continue
		}
		setLeaf(result, parts, value)
	}
	if harness == "codex" {
		if value, found := object.leaf([]string{"payload", "source", "subagent"}); found && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
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
	object, _ := parseSourceObject(body)
	return object.string(path)
}

func UsageRecord(harness, format string, data json.RawMessage) bool {
	object, _ := parseSourceObject(data)
	switch harness {
	case "opencode":
		return format == "opencode-v1" && object.string("data.role") == "assistant" || format == "opencode-v2" && object.string("type") == "assistant"
	case "pi":
		return object.string("type") == "message" && object.string("message.role") == "assistant"
	case "claude-code":
		return object.string("type") == "assistant" && object.string("message.role") == "assistant"
	case "codex":
		return object.string("type") == "event_msg" && object.string("payload.type") == "token_count"
	}
	return false
}

// Scope uses native context for scheduling only; it is not a contribution ID.
func Scope(record Record) string {
	object, _ := parseSourceObject(record.Data)
	return recordScope(record, object)
}

func recordScope(record Record, object *sourceObject) string {
	native := ""
	switch record.Harness {
	case "opencode":
		native = object.string("session_id")
		if record.Format == "opencode-session" {
			native = object.string("id")
		}
	case "claude-code":
		native = object.string("sessionId")
		if native == "" {
			native = object.string("session_id")
		}
	case "pi":
		if object.string("type") == "session" {
			native = object.string("id")
		}
	case "codex":
		if object.string("type") == "session_meta" {
			native = object.string("payload.id")
		}
	}
	if native == "" {
		for _, context := range record.Context {
			if record.Harness != "pi" && record.Harness != "codex" {
				continue
			}
			contextObject, _ := parseSourceObject(context.Data)
			if record.Harness == "pi" && contextObject.string("type") == "session" {
				native = contextObject.string("id")
			}
			if record.Harness == "codex" && contextObject.string("type") == "session_meta" {
				native = contextObject.string("payload.id")
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
	object, _ := parseSourceObject(record.Data)
	scope := recordScope(record, object)
	if strings.HasPrefix(scope, record.Harness+":source:") {
		return ""
	}
	id := ""
	switch record.Harness {
	case "opencode":
		id = object.string("id")
	case "pi":
		id = object.string("id")
	case "claude-code":
		id = object.string("uuid")
		if id == "" {
			id = object.string("message.id") + ":" + object.string("requestId") + ":" + object.string("request_id")
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
