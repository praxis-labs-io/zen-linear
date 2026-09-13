package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praxis-labs-io/zen-linear/internal/config"
)

var ErrCredentialsNotFound = errors.New("credentials not found")

func CredentialsPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadCredentials returns ErrCredentialsNotFound when path does not exist.
func LoadCredentials(path string) (Credentials, error) {
	if path == "" {
		return Credentials{}, fmt.Errorf("credentials path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, ErrCredentialsNotFound
		}
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("parse credentials: %w", err)
	}
	if creds.AccessToken == "" {
		return Credentials{}, fmt.Errorf("credentials missing access_token")
	}
	return creds, nil
}

// SaveCredentials writes creds to path with mode 0600.
func SaveCredentials(path string, creds Credentials) error {
	if path == "" {
		return fmt.Errorf("credentials path is empty")
	}
	if _, err := config.EnsureDirFor(path); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod credentials: %w", err)
	}
	return nil
}

// DeleteCredentials removes path. A missing file is not an error.
func DeleteCredentials(path string) error {
	if path == "" {
		return fmt.Errorf("credentials path is empty")
	}
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete credentials: %w", err)
	}
	return nil
}
