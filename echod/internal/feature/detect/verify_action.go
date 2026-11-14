package detect

import (
	"errors"
	"os"
	"path/filepath"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/feature/security"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

func wakeVerifierAction() *esphome.Action {
	return &esphome.Action{
		Name:    "configure_wake_verifier",
		Args:    []esphome.Arg{{Name: "url", Type: esphome.ArgString}},
		Answers: true,
		Run: func(call esphome.Call) (any, error) {
			if !security.Encrypted() {
				return nil, errors.New("wake verifier configuration requires API encryption")
			}
			endpoint := call.String("url")
			if _, err := parseWakeVerifierURL(endpoint); err != nil {
				return nil, err
			}
			path := filepath.Join(layout.StateDir, "wake-verifier-url")
			if wakeVerifierURL() != endpoint {
				if err := saveWakeVerifier(path, endpoint); err != nil {
					return nil, err
				}
			}
			return map[string]string{"url": endpoint}, nil
		},
	}
}

func saveWakeVerifier(path, endpoint string) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".wake-verifier-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(endpoint + "\n"); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
