//go:build spot && linux

package monitoring

import (
	"errors"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/security"
)

func (f *Feature) Actions() []*esphome.Action {
	return []*esphome.Action{
		{
			Name: "configure_node_exporter",
			Args: []esphome.Arg{
				{Name: "source", Type: esphome.ArgString},
				{Name: "port", Type: esphome.ArgInt},
			},
			Run: func(c esphome.Call) (any, error) {
				if !security.Encrypted() {
					return nil, errors.New("monitoring configuration requires API encryption")
				}
				port := c.Int("port")
				if port < 1 || port > 65535 {
					return nil, errors.New("invalid exporter port")
				}
				return nil, config.Set().SetExporter(config.ExporterSettings{
					Enabled: true, Source: c.String("source"), Port: uint16(port),
				})
			},
		},
		{
			Name: "configure_log_forwarding",
			Args: []esphome.Arg{
				{Name: "collector", Type: esphome.ArgString},
				{Name: "port", Type: esphome.ArgInt},
				{Name: "hostname", Type: esphome.ArgString},
			},
			Run: func(c esphome.Call) (any, error) {
				if !security.Encrypted() {
					return nil, errors.New("monitoring configuration requires API encryption")
				}
				port := c.Int("port")
				if port < 1024 || port > 65535 {
					return nil, errors.New("invalid log collector port")
				}
				return nil, config.Set().SetLogForwarding(config.LogSettings{
					Enabled: true, Collector: c.String("collector"), Port: uint16(port),
					Hostname: c.String("hostname"),
				})
			},
		},
	}
}
