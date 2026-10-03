package service

import (
	"fmt"
	"os"
	"sync"
)

const logLimit = 4 * 1024 * 1024
const logBackups = 3

type rotatingLog struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func openLog(path string) (*rotatingLog, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &rotatingLog{path: path, file: file, size: info.Size()}, nil
}

func (l *rotatingLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	total := 0
	for len(data) > 0 {
		if l.size >= logLimit {
			if err := l.file.Close(); err != nil {
				return total, err
			}
			_ = os.Remove(fmt.Sprintf("%s.%d", l.path, logBackups))
			for i := logBackups - 1; i >= 1; i-- {
				_ = os.Rename(fmt.Sprintf("%s.%d", l.path, i), fmt.Sprintf("%s.%d", l.path, i+1))
			}
			if err := os.Rename(l.path, l.path+".1"); err != nil {
				return total, err
			}
			file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				return total, err
			}
			l.file, l.size = file, 0
		}
		chunk := min(len(data), logLimit-int(l.size))
		n, err := l.file.Write(data[:chunk])
		total += n
		l.size += int64(n)
		data = data[n:]
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (l *rotatingLog) Close() error { l.mu.Lock(); defer l.mu.Unlock(); return l.file.Close() }
