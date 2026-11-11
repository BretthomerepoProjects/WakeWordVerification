//go:build linux && (spot || firmwaremonitoring)

package setup

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/monitoring"
)

func init() { routes["/setup/monitoring"] = (*Feature).saveMonitoring }

func monitoringSection(w http.ResponseWriter, token string) {
	cfg := config.Get().Monitoring
	exporter, logs := monitoring.Status()
	fmt.Fprintf(w, `<fieldset><legend>Node Exporter</legend><p>Status: %s</p>
	<form method="post" action="/setup/monitoring">
	<input type="hidden" name="token" value="%s"><input type="hidden" name="service" value="exporter">
	<label>Prometheus scraper IPv4 address <input name="address" value="%s"></label>
	<label>Listen port <input type="number" name="port" min="1" max="65535" value="%d"></label>
	<button name="enabled" value="1">Save and enable</button><button name="enabled" value="0">Disable</button>
	</form></fieldset>`, html.EscapeString(exporter), html.EscapeString(token),
		html.EscapeString(cfg.Exporter.Source), defaultMonitoringPort(cfg.Exporter.Port, 9100))
	fmt.Fprintf(w, `<fieldset><legend>Log forwarding</legend><p>Status: %s</p>
	<form method="post" action="/setup/monitoring">
	<input type="hidden" name="token" value="%s"><input type="hidden" name="service" value="logs">
	<label>Syslog collector IPv4 address <input name="address" value="%s"></label>
	<label>Collector TCP port <input type="number" name="port" min="1024" max="65535" value="%d"></label>
	<label>Syslog hostname <input name="hostname" maxlength="63" value="%s"></label>
	<button name="enabled" value="1">Save and enable</button><button name="enabled" value="0">Disable</button>
	</form></fieldset>`, html.EscapeString(logs), html.EscapeString(token),
		html.EscapeString(cfg.Logs.Collector), defaultMonitoringPort(cfg.Logs.Port, 1514), html.EscapeString(cfg.Logs.Hostname))
}

func defaultMonitoringPort(port, fallback uint16) uint16 {
	if port == 0 {
		return fallback
	}
	return port
}

func (f *Feature) saveMonitoring(w http.ResponseWriter, r *http.Request) {
	token, in := f.session(r)
	if !in {
		http.Error(w, "not let in", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "post to change monitoring", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := r.ParseForm(); err != nil || !sameToken(r.PostFormValue("token"), token) {
		http.Error(w, "that did not come from this page", http.StatusForbidden)
		return
	}
	f.used()
	enabled := r.PostFormValue("enabled")
	if enabled != "0" && enabled != "1" {
		http.Error(w, "unknown monitoring operation", http.StatusBadRequest)
		return
	}
	port, portErr := strconv.ParseUint(r.PostFormValue("port"), 10, 16)
	if enabled == "1" && (portErr != nil || port == 0) {
		http.Redirect(w, r, back("privacy", "problem", "Enter a valid TCP port."), http.StatusSeeOther)
		return
	}
	cfg := config.Get().Monitoring
	addr := strings.TrimSpace(r.PostFormValue("address"))
	var err error
	switch r.PostFormValue("service") {
	case "exporter":
		cfg.Exporter.Enabled = enabled == "1"
		if cfg.Exporter.Enabled {
			cfg.Exporter.Source, cfg.Exporter.Port = addr, uint16(port)
		}
		err = config.Set().SetExporter(cfg.Exporter)
	case "logs":
		cfg.Logs.Enabled = enabled == "1"
		if cfg.Logs.Enabled {
			cfg.Logs.Collector, cfg.Logs.Port = addr, uint16(port)
			cfg.Logs.Hostname = strings.TrimSpace(r.PostFormValue("hostname"))
		}
		err = config.Set().SetLogForwarding(cfg.Logs)
	default:
		http.Error(w, "unknown monitoring service", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Redirect(w, r, back("privacy", "problem", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/setup?tab=privacy", http.StatusSeeOther)
}
