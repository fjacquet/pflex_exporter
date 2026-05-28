# systemd (EL9 host)

For a non-container deployment on Enterprise Linux 9 — the platform Dell's monitoring
targets — use the unit shipped in `deploy/`.

## Install

```bash
# user + binary
sudo useradd --system --no-create-home --shell /usr/sbin/nologin pflex
sudo install -m 0755 bin/pflex_exporter /usr/local/bin/pflex_exporter

# config + secrets
sudo install -d -o root -g pflex -m 0750 /etc/pflex_exporter
sudo install -m 0640 -o root -g pflex config.yaml /etc/pflex_exporter/config.yaml
sudo install -m 0600 -o root -g pflex deploy/pflex_exporter.env.example /etc/pflex_exporter/pflex_exporter.env
# edit /etc/pflex_exporter/pflex_exporter.env to set FLEX1_PASSWORD=...

# service
sudo install -m 0644 deploy/pflex_exporter.service /etc/systemd/system/pflex_exporter.service
sudo systemctl daemon-reload
sudo systemctl enable --now pflex_exporter
```

Set `logName: ""` in `config.yaml` so logs go to the journal.

## Operate

```bash
journalctl -u pflex_exporter -f         # follow logs
sudo systemctl reload pflex_exporter    # live config reload (sends SIGHUP)
sudo systemctl status pflex_exporter
```

## Hardening

The unit runs as the unprivileged `pflex` user inside a sandbox:

- `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`
- `PrivateTmp`, `PrivateDevices`, `ProtectKernel*`, `ProtectControlGroups`
- `RestrictAddressFamilies=AF_INET AF_INET6`, `RestrictNamespaces`, `LockPersonality`
- `Restart=on-failure`

Secrets are supplied through the `EnvironmentFile` and referenced as `${FLEX1_PASSWORD}`
in `config.yaml`. Keep that file mode `0600`.
