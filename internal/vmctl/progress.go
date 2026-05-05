package vmctl

import (
	"fmt"
	"sync"
	"time"
)

type progressEntry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

var (
	progressMu      sync.Mutex
	progressLog     []progressEntry
	progressMaxSize = 200
)

func addProgress(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	entry := progressEntry{Time: time.Now(), Message: msg}

	progressMu.Lock()
	progressLog = append(progressLog, entry)
	if len(progressLog) > progressMaxSize {
		progressLog = progressLog[len(progressLog)-progressMaxSize:]
	}
	progressMu.Unlock()
}

func getProgressSince(since time.Time) []progressEntry {
	progressMu.Lock()
	defer progressMu.Unlock()

	var result []progressEntry
	for _, e := range progressLog {
		if e.Time.After(since) {
			result = append(result, e)
		}
	}
	return result
}
