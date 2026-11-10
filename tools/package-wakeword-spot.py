#!/usr/bin/env python3
"""Replace the daemon and release marker in an authenticated Spot rootfs."""

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
    parser.add_argument("--base-sha256", required=True, help="digest from the verified official manifest")
    parser.add_argument("--daemon", type=pathlib.Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--out", type=pathlib.Path, required=True)
    args = parser.parse_args()

    if sha256(args.base) != args.base_sha256.lower():
        parser.error("the base does not match the authenticated official rootfs")

    members = {"./usr/local/bin/techo5": "0755", "./etc/techo5-release": "0644"}
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".wakeword-spot-", dir=args.out.parent) as folder:
        work = pathlib.Path(folder)
        raw = work / "rootfs.tar"
        packed = work / "rootfs.tar.gz"
        with gzip.open(args.base, "rb") as source, raw.open("wb") as output:
            shutil.copyfileobj(source, output)
        subprocess.run(["tar", "--delete", "-f", raw, *members], check=True)

        (work / "usr/local/bin").mkdir(parents=True)
        (work / "etc").mkdir()
        shutil.copyfile(args.daemon, work / "./usr/local/bin/techo5")
        (work / "etc/techo5-release").write_text(
            f"techo5 rootfs {args.version} (WakeWordVerification; source {args.revision})\n"
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
            with rootfs.extractfile("./usr/local/bin/techo5") as daemon:
                if hashlib.file_digest(daemon, "sha256").hexdigest() != sha256(args.daemon):
                    raise ValueError("the packaged daemon does not match the build")
            with rootfs.extractfile("./etc/techo5/device.conf") as board:
                if b'T5_PRODUCT="Echo Spot Linux"' not in board.read().splitlines():
                    raise ValueError("the base is not an Echo Spot rootfs")
        packed.replace(args.out)
    print(f"{sha256(args.out)}  {args.out.name}")


if __name__ == "__main__":
    main()
