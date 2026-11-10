package firmware

import (
	"context"
	"log/slog"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
	"github.com/HuskerMinion/techo5/echod/internal/feature/ring"
	"github.com/HuskerMinion/techo5/echod/internal/feature/voice"
	"github.com/HuskerMinion/techo5/echod/internal/update"
)

// Installing by itself (config.Update.AutoInstall), for a device nobody is going to press Install on:
// one with no Home Assistant, in another home. Overnight, when an update's restart and the minute
// without the device are least noticed, and only while it is quiet: nothing playing, nothing ringing.
// A build that does not come up is rolled back by the slot trial as any update is.

const (
	// autoFrom and autoTo are the hours it may install in, local time.
	autoFrom, autoTo = 2, 5

	// autoLook is how often it looks, inside those hours and out.
	autoLook = 15 * time.Minute

	projectAutoLook = 5 * time.Minute
)

func (u *Firmware) buildAuto() {
	u.auto = &esphome.Switch{
		Base: esphome.Base{
			ObjectID: "update_automatically",
			Name:     "Install updates automatically",
			Icon:     "mdi:update",
			Category: esphome.CategoryConfig,
		},
	}
	u.auto.OnCommand = func(on bool) {
		writer := config.Set().Update()
		save := writer.AutoInstall
		if update.ProjectAutoUpdates {
			save = writer.ProjectAutoInstall
		}
		if err := save(on); err != nil {
			slog.Error("saving automatic updates failed", "err", err)
		}
		u.Restore(config.Get())
		slog.Info("updates install automatically", "on", u.AutoInstall())
	}
}

// Restore shows the saved setting.
func (u *Firmware) Restore(c config.Config) { u.auto.Set(autoInstallEnabled(c.Update)) }

// SetAutoInstall is the setting, from the screen or the setup page.
func (u *Firmware) SetAutoInstall(on bool) { u.auto.OnCommand(on) }

// AutoInstall is whether updates install by themselves.
func (u *Firmware) AutoInstall() bool { return autoInstallEnabled(config.Get().Update) }

func autoInstallEnabled(c config.Update) bool {
	if update.ProjectAutoUpdates {
		return c.ProjectAutoInstall
	}
	return c.AutoInstall
}

// autoLoop checks the release feed on its schedule and backs off after a failed install.
func (u *Firmware) autoLoop(ctx context.Context) {
	interval := autoLook
	if update.ProjectAutoUpdates {
		interval = projectAutoLook
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	var tried string // the night last tried, as a date
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now()
		date := now.Format("2006-01-02")
		if !autoDue(u.AutoInstall(), now, tried, autoBusy(), update.ProjectAutoUpdates) {
			continue
		}
		if !update.ProjectAutoUpdates {
			tried = date
		}
		if !update.Published() {
			// A panel's app looks and installs in one go (askApp), and installs nothing that is not newer.
			slog.Info("asking the app to install an update by itself, overnight")
			u.Install(ctx)
			continue
		}
		u.Check(ctx)
		if v := u.Offered(); v != "" && u.AutoInstall() && !autoBusy() {
			tried = date
			slog.Info("installing an update automatically", "version", v)
			u.Install(ctx)
		}
	}
}

func autoBusy() bool {
	playing, _ := media.Get().Playing()
	return playing || ring.IsSounding() || update.Installing() || config.Get().FirstRunPending() ||
		(update.ProjectAutoUpdates && voice.Get().Busy())
}

// autoDue keeps the overnight window for upstream builds. A project opt-in can run while idle at any hour.
func autoDue(on bool, now time.Time, tried string, busy, allowDaytime bool) bool {
	return on && !busy && tried != now.Format("2006-01-02") &&
		(allowDaytime || now.Hour() >= autoFrom && now.Hour() < autoTo)
}
