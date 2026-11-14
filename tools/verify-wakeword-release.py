"""Check a candidate firmware against its last known-good rootfs."""
import argparse
import subprocess
import tarfile


def build_tags(go, daemon):
    result = subprocess.check_output([go, "version", "-m", daemon], text=True)
    for line in result.splitlines():
        setting = line.strip().removeprefix("build\t")
        if setting.startswith("-tags="):
            return set(setting.split("=", 1)[1].strip('"').split(","))
    return set()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for field in ("previous_rootfs", "previous_daemon", "rootfs", "daemon"):
        parser.add_argument(field)
    parser.add_argument("--go", default="go")
    args = parser.parse_args()
    previous_tags = build_tags(args.go, args.previous_daemon)
    tags = build_tags(args.go, args.daemon)
    errors = [f"Missing build tag: {tag}" for tag in sorted(previous_tags - tags)]
    if "spot" not in tags and "firmwaremonitoring" not in tags:
        errors.append("Monitoring supervisor is disabled")
    with tarfile.open(args.previous_rootfs) as previous, tarfile.open(args.rootfs) as candidate:
        entries = {item.name.removeprefix("./"): item for item in candidate}
        for item in previous:
            name = item.name.removeprefix("./")
            current = entries.get(name)
            if current is None:
                errors.append(f"Missing image entry: {name}")
            elif current.type != item.type or (item.mode & 0o111) & ~current.mode:
                errors.append(f"Changed file type or executable permissions: {name}")
        for tool in ("node_exporter", "techo5-log-streamer"):
            item = entries.get(f"usr/local/bin/{tool}")
            if item is None or not item.isfile() or not item.mode & 0o111 or not item.size:
                errors.append(f"Missing executable monitoring payload: {tool}")
    if errors:
        parser.exit(1, "\n".join(errors) + "\n")
    print("Firmware preservation checks passed")


if __name__ == "__main__":
    main()
