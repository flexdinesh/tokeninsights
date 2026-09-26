package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

const jsonlReadBufferBytes = 64 * 1024

// Read only the extent present at open, checking cancellation between chunks.
// Reader handles large records without Scanner's unrecoverable token limit.
type jsonlReader struct {
	ctx      context.Context
	reader   *bufio.Reader
	line     []byte
	err      error
	ended    bool
	deferred bool
	snapshot *sourceSnapshot
}

func newJSONLReader(ctx context.Context, file *os.File) *jsonlReader {
	r := &jsonlReader{ctx: ctx}
	info, err := file.Stat()
	if err != nil {
		r.err = err
		return r
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		r.err = err
		return r
	}
	r.reader = bufio.NewReaderSize(contextReader{ctx: ctx, reader: io.NewSectionReader(file, offset, max(0, info.Size()-offset))}, jsonlReadBufferBytes)
	return r
}

func newSourceJSONLReader(ctx context.Context, file *os.File, source Source, options SyncOptions) *jsonlReader {
	r := newJSONLReader(ctx, file)
	if options.sourceSnapshot != nil && options.sourceSnapshot.source.Path == source.Path {
		offset, err := file.Seek(0, io.SeekCurrent)
		if err == nil && offset == options.sourceSnapshot.size {
			r.snapshot = options.sourceSnapshot
		}
	}
	return r
}

func (r *jsonlReader) Scan() bool {
	if r.err != nil || r.ended {
		return false
	}
	r.line = r.line[:0]
	for {
		if r.err = r.ctx.Err(); r.err != nil {
			return false
		}
		chunk, err := r.reader.ReadSlice('\n')
		if r.snapshot != nil {
			r.snapshot.write(chunk)
		}
		r.line = append(r.line, chunk...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			r.ended = true
			if r.snapshot != nil {
				r.snapshot.complete = true
			}
			// A complete JSON value without a newline is safe to read, but cannot
			// establish an append cursor. An unfinished tail is retried next sync.
			if len(r.line) > 0 && json.Valid(r.line) {
				if r.snapshot != nil && r.snapshot.scanLocations {
					r.snapshot.observeLine(r.line)
				}
				return true
			}
			r.deferred = len(r.line) > 0
			if r.snapshot != nil {
				r.snapshot.deferred = r.deferred
			}
			return false
		}
		if err != nil {
			r.err = err
			return false
		}
		if r.snapshot != nil && r.snapshot.scanLocations {
			r.snapshot.observeLine(r.line)
		}
		return true
	}
}

func (r *jsonlReader) Text() string  { return string(r.line) }
func (r *jsonlReader) Bytes() []byte { return r.line }
func (r *jsonlReader) Err() error    { return r.err }

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := r.reader.Read(p)
	recordSourceBytes(r.ctx, count)
	return count, err
}
