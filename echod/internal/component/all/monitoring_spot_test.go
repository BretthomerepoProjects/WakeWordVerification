//go:build linux && (spot || firmwaremonitoring)

package all

func init() {
	deviceSpecific = append(deviceSpecific,
		"node_exporter", "node_exporter_port", "node_exporter_status", "prometheus_scraper_ip",
		"log_forwarding", "log_forwarding_status", "log_collector_ip", "log_collector_port",
		"log_syslog_hostname", "use_all_cpu_cores",
	)
}
