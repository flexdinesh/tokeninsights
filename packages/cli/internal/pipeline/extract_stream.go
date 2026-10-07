package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"hash"
	"io"
)

type captureLine struct {
	line             []byte
	length           int64
	skipped, partial bool
}

// Oversized lines are validated and drained with bounded memory. Only a proven
// irrelevant complete object can advance the source cursor; native bytes never
// reach the sanitized preparation spool.
func readCaptureLine(ctx context.Context, reader *bufio.Reader, harness Harness, capturedHash hash.Hash) (captureLine, error) {
	var line []byte
	for {
		if err := ctx.Err(); err != nil {
			return captureLine{}, err
		}
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxEvidenceLineBytes {
			marshal, ok := capturedHash.(encoding.BinaryMarshaler)
			if !ok {
				return captureLine{}, errSourceRecordLimit
			}
			savedHash, stateErr := marshal.MarshalBinary()
			if stateErr != nil {
				return captureLine{}, stateErr
			}
			drain := &captureLineReader{ctx: ctx, prefix: bytes.NewReader(line), fragment: fragment, reader: reader, hash: capturedHash, ended: !errors.Is(err, bufio.ErrBufferFull), tail: errors.Is(err, io.EOF), readErr: err}
			discriminator := streamDiscriminator{reader: bufio.NewReader(drain)}
			valid := discriminator.object("", 0) == nil && discriminator.finish() == nil
			// A malformed object may fail early. Still finish the line, never the next
			// one, so a growing tail can be retried without committing a partial hash.
			if _, drainErr := io.Copy(io.Discard, drain); drainErr != nil {
				return captureLine{}, drainErr
			}
			if drain.tail && !valid {
				restore, ok := capturedHash.(encoding.BinaryUnmarshaler)
				if !ok {
					return captureLine{}, errSourceRecordLimit
				}
				if err := restore.UnmarshalBinary(savedHash); err != nil {
					return captureLine{}, err
				}
				return captureLine{length: drain.length, partial: true}, io.EOF
			}
			if !valid || !discriminator.irrelevant(harness) {
				return captureLine{}, errSourceRecordLimit
			}
			if drain.tail {
				return captureLine{length: drain.length, skipped: true}, io.EOF
			}
			return captureLine{length: drain.length, skipped: true}, nil
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return captureLine{line: line, length: int64(len(line))}, err
	}
}

// Returns exactly one source line and hashes the bytes it gives the validator.
// ReadSlice bounds native fragments and cannot consume bytes from the next line.
type captureLineReader struct {
	ctx         context.Context
	prefix      *bytes.Reader
	fragment    []byte
	reader      *bufio.Reader
	hash        hash.Hash
	ended, tail bool
	readErr     error
	length      int64
}

func (r *captureLineReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.prefix.Len() > 0 {
		count, err := r.prefix.Read(body)
		r.length += int64(count)
		_, _ = r.hash.Write(body[:count])
		return count, err
	}
	if len(r.fragment) == 0 {
		if r.ended {
			if r.readErr != nil && !errors.Is(r.readErr, io.EOF) && !errors.Is(r.readErr, bufio.ErrBufferFull) {
				return 0, r.readErr
			}
			return 0, io.EOF
		}
		r.fragment, r.readErr = r.reader.ReadSlice('\n')
		r.ended = !errors.Is(r.readErr, bufio.ErrBufferFull)
		r.tail = errors.Is(r.readErr, io.EOF)
		if len(r.fragment) == 0 {
			return 0, r.readErr
		}
	}
	count := copy(body, r.fragment)
	r.fragment = r.fragment[count:]
	r.length += int64(count)
	_, _ = r.hash.Write(body[:count])
	return count, nil
}

const maxStreamingJSONDepth = 10000
const maxDiscriminatorTokenBytes = 512

var errStreamingJSON = errors.New("invalid_streaming_json")

// Validate the complete JSON grammar, retaining only bounded discriminator
// strings. Unknown/duplicate discriminator paths never establish irrelevance.
type streamDiscriminator struct {
	reader    *bufio.Reader
	values    map[string]string
	seen      map[string]bool
	ambiguous bool
}

func streamSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
func streamHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}
func (s *streamDiscriminator) nonspace() (byte, error) {
	for {
		b, err := s.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		if !streamSpace(b) {
			return b, nil
		}
	}
}
func (s *streamDiscriminator) finish() error {
	for {
		b, err := s.reader.ReadByte()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !streamSpace(b) {
			return errStreamingJSON
		}
	}
}
func (s *streamDiscriminator) string() (string, error) {
	// Opening quote is already consumed. Store at most a small discriminator,
	// including its escapes; all larger strings are syntax checked and drained.
	text := []byte{'"'}
	overflow := false
	remember := func(b byte) {
		if overflow {
			return
		}
		if len(text) >= maxDiscriminatorTokenBytes {
			text = nil
			overflow = true
			return
		}
		text = append(text, b)
	}
	for {
		b, err := s.reader.ReadByte()
		if err != nil {
			return "", err
		}
		remember(b)
		if b < 0x20 {
			return "", errStreamingJSON
		}
		if b == '"' {
			if overflow {
				return "", nil
			}
			var value string
			err := json.Unmarshal(text, &value)
			return value, err
		}
		if b != '\\' {
			continue
		}
		escaped, err := s.reader.ReadByte()
		if err != nil {
			return "", err
		}
		remember(escaped)
		switch escaped {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			for range 4 {
				digit, err := s.reader.ReadByte()
				if err != nil {
					return "", err
				}
				remember(digit)
				if !streamHex(digit) {
					return "", errStreamingJSON
				}
			}
		default:
			return "", errStreamingJSON
		}
	}
}
func discriminatorPath(parent, key string) string {
	if parent == "" {
		switch key {
		case "type", "payload", "message":
			return key
		}
	}
	if parent == "payload" && key == "type" {
		return "payload.type"
	}
	if parent == "message" && key == "role" {
		return "message.role"
	}
	return "?"
}
func (s *streamDiscriminator) object(path string, depth int) error {
	if depth >= maxStreamingJSONDepth {
		return errStreamingJSON
	}
	if depth == 0 {
		b, err := s.nonspace()
		if err != nil {
			return err
		}
		if b != '{' {
			return errStreamingJSON
		}
	}
	b, err := s.nonspace()
	if err != nil {
		return err
	}
	if b == '}' {
		return nil
	}
	for {
		if b != '"' {
			return errStreamingJSON
		}
		key, err := s.string()
		if err != nil {
			return err
		}
		child := discriminatorPath(path, key)
		if child != "?" {
			if s.seen == nil {
				s.seen = make(map[string]bool)
			}
			if s.seen[child] {
				s.ambiguous = true
			}
			s.seen[child] = true
		}
		b, err = s.nonspace()
		if err != nil {
			return err
		}
		if b != ':' {
			return errStreamingJSON
		}
		if err := s.value(child, depth+1); err != nil {
			return err
		}
		b, err = s.nonspace()
		if err != nil {
			return err
		}
		if b == '}' {
			return nil
		}
		if b != ',' {
			return errStreamingJSON
		}
		b, err = s.nonspace()
		if err != nil {
			return err
		}
	}
}
func (s *streamDiscriminator) value(path string, depth int) error {
	if depth > maxStreamingJSONDepth {
		return errStreamingJSON
	}
	b, err := s.nonspace()
	if err != nil {
		return err
	}
	switch b {
	case '"':
		value, err := s.string()
		if err != nil {
			return err
		}
		if path != "?" {
			if s.values == nil {
				s.values = make(map[string]string)
			}
			s.values[path] = value
		}
		return nil
	case '{':
		return s.object(path, depth)
	case '[':
		if depth >= maxStreamingJSONDepth {
			return errStreamingJSON
		}
		b, err := s.nonspace()
		if err != nil {
			return err
		}
		if b == ']' {
			return nil
		}
		if err := s.reader.UnreadByte(); err != nil {
			return err
		}
		for {
			if err := s.value("?", depth+1); err != nil {
				return err
			}
			b, err = s.nonspace()
			if err != nil {
				return err
			}
			if b == ']' {
				return nil
			}
			if b != ',' {
				return errStreamingJSON
			}
		}
	case 't':
		return s.literal("rue")
	case 'f':
		return s.literal("alse")
	case 'n':
		return s.literal("ull")
	default:
		return s.number(b)
	}
}
func (s *streamDiscriminator) literal(tail string) error {
	for i := range len(tail) {
		b, err := s.reader.ReadByte()
		if err != nil {
			return err
		}
		if b != tail[i] {
			return errStreamingJSON
		}
	}
	return nil
}
func (s *streamDiscriminator) number(first byte) error {
	// JSON number grammar checked as a finite state machine, never retained.
	state := 0
	if first == '0' {
		state = 2
	} else if first >= '1' && first <= '9' {
		state = 3
	} else if first != '-' {
		return errStreamingJSON
	}
	for {
		b, err := s.reader.ReadByte()
		if err != nil {
			return err
		}
		digit := b >= '0' && b <= '9'
		next := -1
		switch state {
		case 0:
			if b == '0' {
				next = 2
			} else if b >= '1' && b <= '9' {
				next = 3
			}
		case 2, 3:
			if digit && state == 3 {
				next = 3
			} else if b == '.' {
				next = 4
			} else if b == 'e' || b == 'E' {
				next = 6
			}
		case 4:
			if digit {
				next = 5
			}
		case 5:
			if digit {
				next = 5
			} else if b == 'e' || b == 'E' {
				next = 6
			}
		case 6:
			if b == '+' || b == '-' {
				next = 7
			} else if digit {
				next = 8
			}
		case 7:
			if digit {
				next = 8
			}
		case 8:
			if digit {
				next = 8
			}
		}
		if next >= 0 {
			state = next
			continue
		}
		if state != 2 && state != 3 && state != 5 && state != 8 {
			return errStreamingJSON
		}
		return s.reader.UnreadByte()
	}
}
func (s *streamDiscriminator) irrelevant(harness Harness) bool {
	if s.ambiguous {
		return false
	}
	switch harness {
	case HarnessCodex:
		if s.values["type"] == "response_item" {
			return true
		}
		if s.values["type"] == "event_msg" {
			switch s.values["payload.type"] {
			case "agent_message", "agent_reasoning", "user_message", "exec_command_begin", "exec_command_end", "task_complete", "turn_aborted":
				return true
			}
		}
	case HarnessPi:
		switch s.values["type"] {
		case "model_change", "thinking_level_change", "custom", "compaction", "branch_summary", "label":
			return true
		}
		if s.values["type"] == "message" {
			switch s.values["message.role"] {
			case "user", "toolResult":
				return true
			}
		}
	case HarnessClaudeCode:
		switch s.values["type"] {
		case "user", "system", "progress", "queue-operation", "file-history-snapshot", "summary":
			return true
		}
	}
	return false
}
