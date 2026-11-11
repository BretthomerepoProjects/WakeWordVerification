//go:build spot && linux

package monitoring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
)

const streamerStatus = "/run/techo5-log-streamer/status"

var shared = newFeature()

func init() {
	component.Offer(component.Offered{ID: component.DeviceMonitoring, Suffix: " monitoring"})
	component.Register(component.Network, shared, component.Order(69))
}

type Feature struct {
	exporter service
	logs     service
	entities controls
}

func (f *Feature) Name() string { return "monitoring" }

func (f *Feature) Run(ctx context.Context) error {
	var workers sync.WaitGroup
	workers.Go(func() {
		supervise(ctx, &f.exporter, func() config.ExporterSettings {
			return config.Get().Monitoring.Exporter
		}, func(v config.ExporterSettings) bool { return v.Enabled }, runExporter)
	})
	workers.Go(func() {
		supervise(ctx, &f.logs, func() config.LogSettings {
			return config.Get().Monitoring.Logs
		}, func(v config.LogSettings) bool { return v.Enabled }, runLogs)
	})
	workers.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var previous config.Monitoring
		first := true
		for {
			cfg := config.Get()
			if first || cfg.Monitoring != previous {
				f.Restore(cfg)
				first, previous = false, cfg.Monitoring
			}
			f.entities.exporterStatus.Set(f.exporter.get())
			f.entities.logForwardingStatus.Set(f.logs.get())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	workers.Wait()
	return nil
}

type service struct {
	mu    sync.RWMutex
	state string
}

func (s *service) set(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

func (s *service) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func Status() (exporter, logs string) { return shared.exporter.get(), shared.logs.get() }

func supervise[T comparable](ctx context.Context, svc *service, settings func() T, enabled func(T) bool, run func(context.Context, T, func(string)) error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var current T
	var cancel context.CancelFunc
	var done <-chan error
	var retryAt, started time.Time
	delay := time.Second
	for {
		want := settings()
		if want != current || ctx.Err() != nil {
			if cancel != nil {
				cancel()
				<-done
				cancel, done = nil, nil
			}
			if ctx.Err() != nil {
				return
			}
			current, retryAt, delay = want, time.Time{}, time.Second
		}
		if !enabled(current) {
			svc.set("Disabled")
		} else if done == nil && !time.Now().Before(retryAt) {
			svc.set("Starting")
			childCtx, stop := context.WithCancel(ctx)
			finished := make(chan error, 1)
			cancel, done, started = stop, finished, time.Now()
			go func(v T) { finished <- run(childCtx, v, svc.set) }(current)
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		case err := <-done:
			cancel()
			cancel, done = nil, nil
			if err != nil && ctx.Err() == nil {
				slog.Warn("monitoring service exited", "error", err)
			}
			svc.set("Restarting")
			if time.Since(started) >= time.Minute {
				delay = time.Second
			}
			retryAt = time.Now().Add(delay)
			delay = min(2*delay, time.Minute)
		}
	}
}

func child(ctx context.Context, executable string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	return cmd
}

func runExporter(ctx context.Context, cfg config.ExporterSettings, state func(string)) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	loopback, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	address := loopback.Addr().String()
	_ = loopback.Close()
	public, err := net.Listen("tcp4", ":"+strconv.Itoa(int(cfg.Port)))
	if err != nil {
		return err
	}
	defer public.Close()
	if err := openExporterPort(ctx, cfg); err != nil {
		return err
	}
	defer closeExporterPort()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := child(ctx, "/usr/local/bin/node_exporter", "--web.listen-address="+address,
		"--collector.disable-defaults", "--collector.cpu", "--collector.meminfo",
		"--collector.loadavg", "--collector.netdev", "--collector.filesystem",
		"--collector.uname", "--web.disable-exporter-metrics")
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()

	target := &url.URL{Scheme: "http", Host: address}
	proxy := httputil.NewSingleHostReverseProxy(target)
	source := netip.MustParseAddr(cfg.Source)
	srv := &http.Server{
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       15 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, err := netip.ParseAddrPort(r.RemoteAddr)
			if err != nil || peer.Addr() != source {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if r.Method != http.MethodGet || r.URL.Path != "/metrics" {
				http.NotFound(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		}),
	}
	defer srv.Close()
	served := make(chan error, 1)
	go func() { served <- srv.Serve(public) }()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	return watch(ctx, done, served, func() string {
		resp, err := client.Get(target.String() + "/metrics")
		if err != nil {
			return ""
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return ""
		}
		return "Running"
	}, state)
}

const exporterChain = "TECHO5_METRICS"

func exporterFirewall(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "iptables-legacy", append([]string{"-w", "2"}, args...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("metrics firewall: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func openExporterPort(ctx context.Context, cfg config.ExporterSettings) error {
	_ = exporterFirewall(ctx, "-N", exporterChain)
	if err := exporterFirewall(ctx, "-F", exporterChain); err != nil {
		return err
	}
	jump := []string{"INPUT", "-i", "wlan0", "-j", exporterChain}
	if err := exporterFirewall(ctx, append([]string{"-C"}, jump...)...); err != nil {
		if err := exporterFirewall(ctx, append([]string{"-I"}, jump...)...); err != nil {
			return err
		}
	}
	return exporterFirewall(ctx, "-A", exporterChain, "-s", cfg.Source+"/32",
		"-p", "tcp", "--dport", strconv.Itoa(int(cfg.Port)), "-j", "ACCEPT")
}

func closeExporterPort() {
	if err := exporterFirewall(context.Background(), "-F", exporterChain); err != nil {
		slog.Warn("could not close the metrics firewall rule", "error", err)
	}
}

func runLogs(ctx context.Context, cfg config.LogSettings, state func(string)) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll("/run/techo5-log-streamer", 0700); err != nil {
		return err
	}
	_ = os.Remove(streamerStatus)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := child(ctx, "/usr/local/bin/techo5-log-streamer", cfg.Collector,
		strconv.Itoa(int(cfg.Port)), cfg.Hostname, "screen", streamerStatus)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()
	return watch(ctx, done, nil, func() string {
		data, err := os.ReadFile(streamerStatus)
		if err != nil {
			return ""
		}
		if strings.Contains("\n"+string(data), "\nconnected=1\n") {
			return "Running"
		}
		return "Running · collector disconnected"
	}, state)
}

func watch(ctx context.Context, child chan error, server <-chan error, check func() string, state func(string)) error {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-child:
			child <- err // Leave the wait result for the owner to reap.
			return fmt.Errorf("child exited: %v", err)
		case err := <-server:
			return err
		case <-tick.C:
			status := check()
			if status == "" {
				return errors.New("monitoring service did not respond")
			}
			state(status)
		}
	}
}
