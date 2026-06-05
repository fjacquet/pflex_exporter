# Installation

## Prerequisites

- A reachable PowerFlex cluster gateway — **Gen1 4.5+** or **Gen2 5.x+** (primary ingress IP for 4.x+).
- A PowerFlex user with **monitor** privileges (read-only is sufficient).
- One of: Go 1.25+ toolchain (build from source), Docker, or a Kubernetes cluster.

## With Homebrew

```bash
brew install --cask fjacquet/tap/pflex_exporter
```

Works on macOS and Linuxbrew. Published from the `fjacquet/homebrew-tap` tap on each
release.

## From a release archive

Download the `tar.gz` for your platform from the
[releases page](https://github.com/fjacquet/pflex_exporter/releases), verify it against
`checksums.txt`, then extract and install:

```bash
sha256sum -c checksums.txt --ignore-missing
tar -xzf pflex_exporter_*_linux_amd64.tar.gz
sudo install pflex_exporter /usr/local/bin/pflex_exporter
pflex_exporter --version
```

Each release also ships a CycloneDX SBOM (`pflex_exporter_<version>.sbom.cdx.json`).

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
docker pull ghcr.io/fjacquet/pflex_exporter:0.3.0   # or :latest
```

## Next steps

- [Configure](configuration.md) your clusters and exporters.
- [Quick Start](quickstart.md) to run it.
- Deploy via [Docker](../deployment/docker.md), [systemd](../deployment/systemd.md) or [Kubernetes](../deployment/kubernetes.md).
