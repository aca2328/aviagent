package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// sessionRetention is how long a session survives before List and Get treat
// it as expired and remove its file.
const sessionRetention = 30 * 24 * time.Hour

// sessionTitleMaxLen bounds the rail's session title, derived from the first
// user message.
const sessionTitleMaxLen = 60

// SessionSummary is the lightweight view List returns for the session rail —
// no need to read every message in every file just to show a title and date.
type SessionSummary struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Model   string    `json:"model"`
	Created time.Time `json:"created"`
}

// SessionStore persists chat sessions. Implementations must be safe for
// concurrent use.
type SessionStore interface {
	Create(model string) (*ChatSession, error)
	Get(id string) (*ChatSession, error)
	List() ([]SessionSummary, error)
	AppendMessage(id string, msg ChatMessage) error
	Delete(id string) error
	DeleteAll() error
}

// sessionHeader is the first line of a session's JSONL file.
type sessionHeader struct {
	ID      string    `json:"id"`
	Model   string    `json:"model"`
	Created time.Time `json:"created"`
}

// JSONLSessionStore stores each session as its own JSONL file: a header line
// followed by one line per ChatMessage, appended as the conversation grows.
// One file per session keeps appends cheap (no read-modify-write of a shared
// file) and makes 30-day expiry a matter of stat'ing and removing individual
// files.
type JSONLSessionStore struct {
	dir string
}

func NewJSONLSessionStore(dir string) (*JSONLSessionStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}
	return &JSONLSessionStore{dir: dir}, nil
}

func (s *JSONLSessionStore) path(id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("invalid session id: %w", err)
	}
	return filepath.Join(s.dir, id+".jsonl"), nil
}

func (s *JSONLSessionStore) Create(model string) (*ChatSession, error) {
	session := &ChatSession{
		ID:      uuid.New().String(),
		Model:   model,
		Created: time.Now(),
	}

	path, err := s.path(session.ID)
	if err != nil {
		return nil, err
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create session file: %w", err)
	}
	defer file.Close()

	header := sessionHeader{ID: session.ID, Model: session.Model, Created: session.Created}
	data, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session header: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write session header: %w", err)
	}

	return session, nil
}

func (s *JSONLSessionStore) AppendMessage(id string, msg ChatMessage) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal chat message: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to append chat message: %w", err)
	}

	return nil
}

func (s *JSONLSessionStore) Get(id string) (*ChatSession, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	if !scanner.Scan() {
		return nil, fmt.Errorf("session file is empty")
	}
	var header sessionHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("failed to parse session header: %w", err)
	}

	if time.Since(header.Created) > sessionRetention {
		os.Remove(path)
		return nil, fmt.Errorf("session expired")
	}

	session := &ChatSession{ID: header.ID, Model: header.Model, Created: header.Created}
	for scanner.Scan() {
		var msg ChatMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip a corrupt line rather than failing the whole session
		}
		session.Messages = append(session.Messages, msg)
	}

	return session, nil
}

func (s *JSONLSessionStore) List() ([]SessionSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var summaries []SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())

		header, firstMessage, err := readSessionHead(path)
		if err != nil {
			continue // skip files we can't parse rather than failing the whole list
		}

		if time.Since(header.Created) > sessionRetention {
			os.Remove(path)
			continue
		}

		title := "New session"
		if firstMessage != "" {
			title = firstMessage
			if len(title) > sessionTitleMaxLen {
				title = title[:sessionTitleMaxLen-1] + "…"
			}
		}

		summaries = append(summaries, SessionSummary{
			ID:      header.ID,
			Title:   title,
			Model:   header.Model,
			Created: header.Created,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Created.After(summaries[j].Created)
	})

	return summaries, nil
}

// readSessionHead reads just the header line and the first message's content
// (for the rail's title) without loading a whole session's history.
func readSessionHead(path string) (sessionHeader, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return sessionHeader{}, "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !scanner.Scan() {
		return sessionHeader{}, "", fmt.Errorf("empty session file")
	}
	var header sessionHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return sessionHeader{}, "", err
	}

	firstMessage := ""
	if scanner.Scan() {
		var msg ChatMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil {
			firstMessage = msg.Content
		}
	}

	return header, firstMessage, nil
}

func (s *JSONLSessionStore) Delete(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

func (s *JSONLSessionStore) DeleteAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		os.Remove(filepath.Join(s.dir, entry.Name()))
	}
	return nil
}
