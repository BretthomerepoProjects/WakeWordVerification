package security

import (
	"errors"
	"slices"
	"strings"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	esphome "github.com/ygelfand/go-esphome-device"
)

func (f *Feature) enableSSHWithKeyAction() *esphome.Action {
	return &esphome.Action{
		Name: "enable_ssh_with_key",
		Args: []esphome.Arg{{Name: "public_key", Type: esphome.ArgString}},
		Run: func(c esphome.Call) (any, error) {
			if !encrypted() {
				return nil, errors.New("SSH enrollment requires API encryption")
			}
			return nil, f.enrollSSHKey(c.String("public_key"), func() error { return config.Set().Security().SSH(true) })
		},
	}
}

// EnableLocalSSH enrolls a key from an authorized device setup session.
func (f *Feature) EnableLocalSSH(publicKey string) error {
	if !sshAvailable() {
		return errors.New("SSH is unavailable on this image")
	}
	return f.enrollSSHKey(publicKey, config.Set().Security().EnableLocalSSH)
}

func (f *Feature) enrollSSHKey(publicKey string, enable func() error) error {
	keys, err := parseKeys(publicKey)
	if err != nil {
		return err
	}
	if len(keys) != 1 {
		return errors.New("provide exactly one SSH public key")
	}

	f.sshKeysMu.Lock()
	defer f.sshKeysMu.Unlock()
	existing := readKeys()
	keyFields := strings.Fields(keys[0])
	if !slices.ContainsFunc(existing, func(line string) bool {
		fields := strings.Fields(line)
		return len(fields) >= 2 && fields[0] == keyFields[0] && fields[1] == keyFields[1]
	}) {
		adoptManaged()
		existing = append(existing, keys[0])
		if err := writeKeys(existing); err != nil {
			return err
		}
	}
	ensureDropbearSees(existing)
	if err := enable(); err != nil {
		return err
	}
	f.ssh.Set(true)
	f.Changed.Emit(struct{}{})
	select {
	case f.wake <- struct{}{}:
	default:
	}
	return nil
}
