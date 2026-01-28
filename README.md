# tailwire

Watch log files, POST matching lines to webhook. Uses inotify for instant detection.

## Build

```bash
# Requires Go 1.21+
go build -o tailwire main.go

# Or cross-compile for Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tailwire main.go
```

## Usage

```bash
tailwire -file /var/log/exim/mainlog -pattern "Mail delivery failed" -webhook https://example.com/webhook
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-file` | required | Log file to watch |
| `-pattern` | required | Regex pattern to match |
| `-webhook` | required | Webhook URL |

## Webhook payload

```json
{
  "hostname": "mail.example.com",
  "line": "2026-01-28 21:15:44 1vlBx2-000000057fX-2hji Mail delivery failed..."
}
```

## Systemd

Install binary:

```bash
cp tailwire /usr/local/bin/
chmod +x /usr/local/bin/tailwire
```

Create `/etc/systemd/system/tailwire.service`:

```ini
[Unit]
Description=Log Webhook Monitor
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/tailwire \
    -file /var/log/exim/mainlog \
    -pattern "Mail delivery failed" \
    -webhook https://example.com/webhook
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
systemctl daemon-reload
systemctl enable --now tailwire
systemctl status tailwire
journalctl -u tailwire -f
```

## Multiple instances

```bash
cp tailwire.service /etc/systemd/system/tailwire-exim.service
cp tailwire.service /etc/systemd/system/tailwire-nginx.service
# Edit each with different -file and -pattern
```

## Behavior

- Starts from end of file (no duplicates on restart)
- Handles log rotation automatically
- Logs webhook errors to stderr
