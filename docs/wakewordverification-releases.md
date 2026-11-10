# WakeWordVerification releases

WakeWordVerification carries the shared TECHO5 daemon and its upstream history. See [LICENSE](../LICENSE) and [NOTICE](../NOTICE). The first project update target is the Echo Spot 1st generation (`rook`). Build it with Go 1.27.3, `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0`, and `-tags spot,wakewordverification`. Set the daemon's `internal/layout.Version` to the manifest version and record the source revision in `internal/layout.GitCommit`.

The project build checks [WakeWordVerification releases](https://github.com/BretthomerepoProjects/WakeWordVerification/releases) and verifies its manifest with the project Ed25519 public key. Use one published, versioned release tagged `wakeword-spot-<version>` as GitHub's latest release. The firmware update entity on each Home Assistant device page can install a later release. A verified rootfs is downloaded, checked against the signed size and SHA-256, then installed in the inactive slot.

On builds with project automatic updates, the Home Assistant switch **Install updates automatically** is off until enabled on that device. The opt-in is independent of the upstream setting. While idle, opted-in devices check the signed feed every five minutes after the initial boot health check. Automatic installs wait during playback, alarms, Assist turns, and first-run setup.

For a daemon-only change, build the ARM daemon and run `tools/package-wakeword-rootfs.py` under Linux or WSL. Provide the board family, official rootfs, its digest from the verified official manifest, the new daemon, its version and revision, and the output path. The tool keeps the other rootfs files and metadata intact. Rebuild the full rootfs when changing its packages or helper binaries. Keep device-specific vendor and recovery backups private.

## Device update feeds

| Device | Release feed | Rootfs key | Build tags |
| --- | --- | --- | --- |
| Echo Spot | [Latest Spot release](https://github.com/BretthomerepoProjects/WakeWordVerification/releases/latest) | `arm-spot` | `spot,wakewordverification` |
| Echo Show 5 | [Show 5 stable](https://github.com/BretthomerepoProjects/WakeWordVerification/releases/tag/wakeword-show5-stable) | `arm` | `wakewordverification` |
| Echo Show 8, first generation | [Show 8 stable](https://github.com/BretthomerepoProjects/WakeWordVerification/releases/tag/wakeword-show8-stable) | `arm` | `wakewordverification` |
| Echo Dot, second generation | [Dot stable](https://github.com/BretthomerepoProjects/WakeWordVerification/releases/tag/wakeword-dot-stable) | `arm-dot` | `dot,wakewordverification` |

The Show 5 and first-generation Show 8 share the `arm` artifact. Each feed carries its own signed manifest. Keep the Spot release marked latest, as its existing builds use GitHub's latest release endpoint.

Use `echod/cmd/mkmanifest` with `-arm-spot` and `-rootfs-arm-spot` to describe the release files. Set `-from` to the versioned GitHub release download URL, and use `-sign-key -` to supply the project seed through stdin from its protected store. Publish the daemon, rootfs, `manifest.json`, and `manifest.json.sig` together. Attach the corresponding third-party sources from the verified base release, including the build instructions and notices, when repackaging upstream binaries. Verify the published manifest and the downloaded rootfs before installing.

The first project build is installed locally on the designated Spot after host verification because the official updater trusts the upstream key. Subsequent builds use the normal manual update flow. To return to official TECHO5, use the authenticated upstream recovery assets and the device's private backups.
