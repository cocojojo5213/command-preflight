# Contributing

Run the local checks before opening a pull request:

```bash
go test ./...
go vet ./...
sh scripts/check-skill-sync.sh
```

Add regression cases for every shell or privacy bug. Tests must not contain real credentials, private paths, or customer command output.

The default product behavior is offline. Changes that add network access, telemetry, or command execution need an explicit design note and privacy/security review.
