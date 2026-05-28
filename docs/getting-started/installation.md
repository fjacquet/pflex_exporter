# Installation

## Prerequisites

- A reachable PowerFlex cluster gateway — **Gen1 4.5+** or **Gen2 5.x+** (primary ingress IP for 4.x+).
- A PowerFlex user with **monitor** privileges (read-only is sufficient).
- One of: Go 1.25+ toolchain (build from source), Docker, or a Kubernetes cluster.

## From a release binary

Download the binary for your platform from the
[releases page](https://github.com/fjacquet/pflex_exporter/releases) and verify it
against `checksums.txt`:

```bash
sha256sum -c checksums.txt --ignore-missing
chmod +x pflex_exporter_*_linux_amd64
sudo install pflex_exporter_*_linux_amd64 /usr/local/bin/pflex_exporter
pflex_exporter --version
```

Each release also ships a CycloneDX SBOM (`sbom.cdx.json`).

## From source

```bash
git clone https://github.com/fjacquet/pflex_exporter
cd pflex_exporter
make cli            # -> bin/pflex_exporter
```

## Container image

Multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR with SBOM and
provenance attestations:

```bash
docker pull ghcr.io/fjacquet/pflex_exporter:0.1.0   # or :latest
```

## Next steps

- [Configure](configuration.md) your clusters and exporters.
- [Quick Start](quickstart.md) to run it.
- Deploy via [Docker](../deployment/docker.md), [systemd](../deployment/systemd.md) or [Kubernetes](../deployment/kubernetes.md).
