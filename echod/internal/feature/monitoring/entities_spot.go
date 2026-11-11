//go:build spot && linux

package monitoring

import (
	"log/slog"
	"strings"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/security"
)

type controls struct {
	exporter, logs                      *esphome.Switch
	scraper, collector, hostname        *esphome.Text
	exporterPort, collectorPort         *esphome.Number
	exporterStatus, logForwardingStatus *esphome.TextSensor
}

func newFeature() *Feature {
	f := &Feature{exporter: service{state: "Disabled"}, logs: service{state: "Disabled"}}
	e := &f.entities
	base := func(id, name string) esphome.Base {
		return esphome.Base{ObjectID: id, Name: name, Category: esphome.CategoryConfig,
			DeviceID: component.DeviceMonitoring}
	}
	port := func(id, name string, minimum float64, command func(float32)) *esphome.Number {
		return &esphome.Number{Base: base(id, name), Min: minimum, Max: 65535, Step: 1,
			Mode: esphome.NumberBox, OnCommand: command}
	}
	e.exporter = &esphome.Switch{Base: base("node_exporter", "Node Exporter"),
		OnCommand: func(on bool) { f.editExporter(func(v *config.ExporterSettings) { v.Enabled = on }) }}
	e.logs = &esphome.Switch{Base: base("log_forwarding", "Log forwarding"),
		OnCommand: func(on bool) { f.editLogs(func(v *config.LogSettings) { v.Enabled = on }) }}
	e.scraper = &esphome.Text{Base: base("prometheus_scraper_ip", "Prometheus scraper IPv4 address"), MaxLength: 15,
		OnCommand: func(s string) { f.editExporter(func(v *config.ExporterSettings) { v.Source = strings.TrimSpace(s) }) }}
	e.collector = &esphome.Text{Base: base("log_collector_ip", "Log collector IPv4 address"), MaxLength: 15,
		OnCommand: func(s string) { f.editLogs(func(v *config.LogSettings) { v.Collector = strings.TrimSpace(s) }) }}
	e.hostname = &esphome.Text{Base: base("log_syslog_hostname", "Syslog hostname"), MaxLength: 63,
		OnCommand: func(s string) { f.editLogs(func(v *config.LogSettings) { v.Hostname = strings.TrimSpace(s) }) }}
	e.exporterPort = port("node_exporter_port", "Node Exporter port", 1, func(p float32) {
		if p >= 1 && p <= 65535 {
			f.editExporter(func(v *config.ExporterSettings) { v.Port = uint16(p) })
		}
	})
	e.collectorPort = port("log_collector_port", "Log collector port", 1024, func(p float32) {
		if p >= 1024 && p <= 65535 {
			f.editLogs(func(v *config.LogSettings) { v.Port = uint16(p) })
		}
	})
	e.exporterStatus = &esphome.TextSensor{Base: esphome.Base{ObjectID: "node_exporter_status",
		Name: "Node Exporter status", Category: esphome.CategoryDiagnostic, DeviceID: component.DeviceMonitoring}}
	e.logForwardingStatus = &esphome.TextSensor{Base: esphome.Base{ObjectID: "log_forwarding_status",
		Name: "Log forwarding status", Category: esphome.CategoryDiagnostic, DeviceID: component.DeviceMonitoring}}
	return f
}

func (f *Feature) Entities() []esphome.Entity {
	e := &f.entities
	return []esphome.Entity{e.exporter, e.scraper, e.exporterPort, e.exporterStatus,
		e.logs, e.collector, e.collectorPort, e.hostname, e.logForwardingStatus}
}

func (f *Feature) Restore(c config.Config) {
	e := &f.entities
	ex, logs := c.Monitoring.Exporter, c.Monitoring.Logs
	e.exporter.Set(ex.Enabled)
	e.scraper.Set(ex.Source)
	e.exporterPort.Set(float32(defaultPort(ex.Port, 9100)))
	e.logs.Set(logs.Enabled)
	e.collector.Set(logs.Collector)
	e.collectorPort.Set(float32(defaultPort(logs.Port, 1514)))
	e.hostname.Set(logs.Hostname)
	e.exporterStatus.Set(f.exporter.get())
	e.logForwardingStatus.Set(f.logs.get())
}

func defaultPort(port, fallback uint16) uint16 {
	if port == 0 {
		return fallback
	}
	return port
}

func (f *Feature) editExporter(edit func(*config.ExporterSettings)) {
	if !security.Encrypted() {
		return
	}
	v := config.Get().Monitoring.Exporter
	v.Port = defaultPort(v.Port, 9100)
	edit(&v)
	if err := config.Set().SetExporter(v); err != nil {
		slog.Warn("Node Exporter setting refused", "error", err)
		f.entities.exporterStatus.Set(err.Error())
	}
}

func (f *Feature) editLogs(edit func(*config.LogSettings)) {
	if !security.Encrypted() {
		return
	}
	v := config.Get().Monitoring.Logs
	v.Port = defaultPort(v.Port, 1514)
	edit(&v)
	if err := config.Set().SetLogForwarding(v); err != nil {
		slog.Warn("log forwarding setting refused", "error", err)
		f.entities.logForwardingStatus.Set(err.Error())
	}
}
