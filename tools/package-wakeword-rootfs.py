#!/usr/bin/env python3
"""Replace the daemon and release marker in an authenticated TECHO5 rootfs."""

import argparse
import gzip
import hashlib
import pathlib
import shutil
import subprocess
import tarfile
import tempfile


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=pathlib.Path, required=True)
    parser.add_argument("--board", choices=("spot", "show", "dot"), default="spot")
    parser.add_argument("--base-sha256", required=True, help="digest from the verified official manifest")
    parser.add_argument("--daemon", type=pathlib.Path, required=True)
    parser.add_argument("--node-exporter", type=pathlib.Path)
    parser.add_argument("--log-streamer", type=pathlib.Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--out", type=pathlib.Path, required=True)
    args = parser.parse_args()
    if args.node_exporter or args.log_streamer:
        if not (args.node_exporter and args.log_streamer):
            parser.error("firmware monitoring requires both payloads")

    if sha256(args.base) != args.base_sha256.lower():
        parser.error("the base does not match the authenticated official rootfs")

    if args.board == "dot":
        daemon_path, release_path, marker = "usr/local/bin/echod", "etc/techo5-release", "techo5-dot"
    else:
        daemon_path, release_path, marker = "./usr/local/bin/techo5", "./etc/techo5-release", "techo5 rootfs"
    members = {daemon_path: "0755", release_path: "0644"}
    payloads = {daemon_path: args.daemon}
    if args.node_exporter:
        bin_dir = daemon_path.rsplit("/", 1)[0]
        payloads.update({
            f"{bin_dir}/node_exporter": args.node_exporter,
            f"{bin_dir}/techo5-log-streamer": args.log_streamer,
        })
        members.update({name: "0755" for name in payloads})
    with tarfile.open(args.base) as base:
        with base.extractfile(release_path) as release:
            if not release.read().startswith(f"{marker} ".encode()):
                raise ValueError("the base release marker does not match the requested target")
        has_spot_marker = "./etc/techo5/device.conf" in base.getnames()
        old_members = [name for name in members if name in base.getnames()]
        if args.board == "spot":
            with base.extractfile("./etc/techo5/device.conf") as board:
                if b'T5_PRODUCT="Echo Spot Linux"' not in board.read().splitlines():
                    raise ValueError("the base is not an Echo Spot rootfs")
        elif args.board == "show" and has_spot_marker:
            raise ValueError("the shared Show base must not have a board-specific device.conf")
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".wakeword-spot-", dir=args.out.parent) as folder:
        work = pathlib.Path(folder)
        raw = work / "rootfs.tar"
        packed = work / "rootfs.tar.gz"
        with gzip.open(args.base, "rb") as source, raw.open("wb") as output:
            shutil.copyfileobj(source, output)
        subprocess.run(["tar", "--delete", "-f", raw, *old_members], check=True)

        (work / "usr/local/bin").mkdir(parents=True)
        (work / "etc").mkdir()
        for name, payload in payloads.items():
            shutil.copyfile(payload, work / name)
        (work / release_path).write_text(
            f"{marker} {args.version} (WakeWordVerification; source {args.revision})\n"
        )
        for name, mode in members.items():
            subprocess.run(
                ["tar", "--append", "--format=gnu", "--owner=0", "--group=0", f"--mode={mode}",
                 "--mtime=@0", "-f", raw, "-C", work, name],
                check=True,
            )

        with packed.open("wb") as output, raw.open("rb") as source:
            with gzip.GzipFile(filename="", fileobj=output, mode="wb", mtime=0) as compressed:
                shutil.copyfileobj(source, compressed)
        with tarfile.open(packed) as rootfs:
            names = rootfs.getnames()
            if any(names.count(name) != 1 for name in members):
                raise ValueError("each replacement must appear exactly once in the rootfs")
            for name, payload in payloads.items():
                with rootfs.extractfile(name) as archived:
                    if hashlib.file_digest(archived, "sha256").hexdigest() != sha256(payload):
                        raise ValueError(f"the packaged {name} does not match its input")
        packed.replace(args.out)
    print(f"{sha256(args.out)}  {args.out.name}")


if __name__ == "__main__":
    main()
