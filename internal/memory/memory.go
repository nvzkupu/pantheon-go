package memory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zkupu/pantheon/internal/gateway"
)

// Store is the interface for conversation persistence.
type Store interface {
	Save(sessionID string, messages []gateway.Message) error
	Load(sessionID string) ([]gateway.Message, error)
	List() ([]SessionInfo, error)
}

type SessionInfo struct {
	ID        string    `json:"id"`
	Agent     string    `json:"agent"`
	Messages  int       `json:"messages"`
	UpdatedAt time.Time `json:"updated_at"`
}

type session struct {
	ID        string            `json:"id"`
	Agent     string            `json:"agent"`
	Messages  []gateway.Message `json:"messages"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// FileStore persists sessions as JSON files in a directory.
type FileStore struct {
	Dir string
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}
	return &FileStore{Dir: dir}, nil
}

func (fs *FileStore) path(id string) string {
	return filepath.Join(fs.Dir, id+".json")
}

func (fs *FileStore) Save(id string, messages []gateway.Message) error {
	s := session{ID: id, Messages: messages, UpdatedAt: time.Now()}

	if existing, err := fs.loadSession(id); err == nil {
		s.CreatedAt = existing.CreatedAt
		s.Agent = existing.Agent
	} else {
		s.CreatedAt = time.Now()
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return os.WriteFile(fs.path(id), data, 0o644)
}

func (fs *FileStore) Load(id string) ([]gateway.Message, error) {
	s, err := fs.loadSession(id)
	if err != nil {
		return nil, err
	}
	return s.Messages, nil
}

func (fs *FileStore) loadSession(id string) (*session, error) {
	data, err := os.ReadFile(fs.path(id))
	if err != nil {
		return nil, err
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &s, nil
}

func (fs *FileStore) List() ([]SessionInfo, error) {
	entries, err := os.ReadDir(fs.Dir)
	if err != nil {
		return nil, err
	}
	var infos []SessionInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		s, err := fs.loadSession(id)
		if err != nil {
			continue
		}
		infos = append(infos, SessionInfo{
			ID: s.ID, Agent: s.Agent,
			Messages: len(s.Messages), UpdatedAt: s.UpdatedAt,
		})
	}
	return infos, nil
}

// SessionID generates a deterministic ID from agent name and a label.
func SessionID(agentName, label string) string {
	h := sha256.Sum256([]byte(agentName + ":" + label))
	return fmt.Sprintf("%x", h[:8])
}

// WindowTrimmer keeps only the last N message pairs plus the system prompt.
type WindowTrimmer struct {
	MaxPairs int
}

func (w *WindowTrimmer) Trim(messages []gateway.Message) []gateway.Message {
	convStart := 0
	for i, m := range messages {
		if m.Role == "user" {
			convStart = i
			break
		}
	}

	prefix := messages[:convStart]
	conv := messages[convStart:]

	maxMsgs := w.MaxPairs * 2
	if len(conv) <= maxMsgs {
		return messages
	}

	trimmed := make([]gateway.Message, 0, len(prefix)+maxMsgs)
	trimmed = append(trimmed, prefix...)
	trimmed = append(trimmed, conv[len(conv)-maxMsgs:]...)
	return trimmed
}

// SummaryCompressor compresses older turns into a summary when conversation
// exceeds a threshold. Requires a summarizer function (typically an LLM call).
type SummaryCompressor struct {
	ThresholdPairs int
	KeepPairs      int
	Summarize      func(messages []gateway.Message) (string, error)
}

func (sc *SummaryCompressor) Compress(messages []gateway.Message) ([]gateway.Message, error) {
	convStart := 0
	for i, m := range messages {
		if m.Role == "user" {
			convStart = i
			break
		}
	}

	prefix := messages[:convStart]
	conv := messages[convStart:]

	if len(conv)/2 <= sc.ThresholdPairs {
		return messages, nil
	}

	keepMsgs := sc.KeepPairs * 2
	toSummarize := conv[:len(conv)-keepMsgs]
	toKeep := conv[len(conv)-keepMsgs:]

	summary, err := sc.Summarize(toSummarize)
	if err != nil {
		return messages, nil
	}

	result := make([]gateway.Message, 0, len(prefix)+1+len(toKeep))
	result = append(result, prefix...)
	result = append(result, gateway.Message{
		Role:    "system",
		Content: fmt.Sprintf("[Conversation summary: %s]", summary),
	})
	result = append(result, toKeep...)
	return result, nil
}
