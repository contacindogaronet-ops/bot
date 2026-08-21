package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/gotd/td/session"
	"github.com/rs/zerolog"
)

// FileStorage implements session.Storage using an atomic local file persistence layer.
type FileStorage struct {
	path string
	mux  sync.Mutex
	log  zerolog.Logger
}

// NewFileStorage creates a new thread-safe, persistent session storage at the specified path.
func NewFileStorage(path string, log zerolog.Logger) (*FileStorage, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}

	return &FileStorage{
		path: path,
		log:  log.With().Str("component", "session_storage").Logger(),
	}, nil
}

// LoadSession reads the saved session bytes from disk.
func (f *FileStorage) LoadSession(ctx context.Context) ([]byte, error) {
	f.mux.Lock()
	defer f.mux.Unlock()

	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			f.log.Debug().Str("path", f.path).Msg("No existing session found; will perform fresh authentication")
			return nil, session.ErrNotFound
		}
		f.log.Error().Err(err).Str("path", f.path).Msg("Failed to read session file")
		return nil, err
	}

	if len(data) == 0 {
		return nil, session.ErrNotFound
	}

	f.log.Info().Str("path", f.path).Int("bytes", len(data)).Msg("Loaded existing persistent Telegram session")
	return data, nil
}

// StoreSession atomically writes the updated session bytes to disk to prevent corruption on sudden power-off/kill.
func (f *FileStorage) StoreSession(ctx context.Context, data []byte) error {
	f.mux.Lock()
	defer f.mux.Unlock()

	tmpPath := f.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		f.log.Error().Err(err).Str("tmp_path", tmpPath).Msg("Failed to write temporary session file")
		return err
	}

	if err := os.Rename(tmpPath, f.path); err != nil {
		f.log.Error().Err(err).Str("path", f.path).Msg("Failed to rename temporary session to target path")
		return err
	}

	f.log.Debug().Str("path", f.path).Int("bytes", len(data)).Msg("Persisted session auth keys to disk successfully")
	return nil
}
