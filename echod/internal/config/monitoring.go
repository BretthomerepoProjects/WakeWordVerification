package config

import (
	"errors"
	"net/netip"
	"regexp"
)

type Monitoring struct {
	Exporter ExporterSettings `json:"exporter"`
	Logs     LogSettings      `json:"logs"`
}

type ExporterSettings struct {
	Enabled bool   `json:"enabled"`
	Source  string `json:"source,omitempty"`
	Port    uint16 `json:"port,omitempty"`
}

type LogSettings struct {
	Enabled   bool   `json:"enabled"`
	Collector string `json:"collector,omitempty"`
	Port      uint16 `json:"port,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
}

var logHostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

func (v ExporterSettings) Validate() error {
	if !v.Enabled {
		return nil
	}
	if !monitoringIPv4(v.Source) || v.Port == 0 {
		return errors.New("enter the Prometheus scraper IPv4 address and a nonzero listen port")
	}
	return nil
}

func (v LogSettings) Validate() error {
	if !v.Enabled {
		return nil
	}
	if !monitoringIPv4(v.Collector) || v.Port < 1024 {
		return errors.New("enter the syslog collector IPv4 address and a TCP port from 1024 to 65535")
	}
	if !logHostname.MatchString(v.Hostname) {
		return errors.New("enter a syslog hostname of up to 63 letters, digits, dots, underscores, or hyphens")
	}
	return nil
}

func monitoringIPv4(value string) bool {
	addr, err := netip.ParseAddr(value)
	return err == nil && addr.Is4() && addr.IsGlobalUnicast()
}

func (w Writer) SetExporter(v ExporterSettings) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return w.st.Update(func(c *Config) { c.Monitoring.Exporter = v })
}

func (w Writer) SetLogForwarding(v LogSettings) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return w.st.Update(func(c *Config) { c.Monitoring.Logs = v })
}
