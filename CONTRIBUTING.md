# Contributing

Use Go 1.26.5 or newer and run `make check`.

Changes to the network boundary must include tests proving the exact method/host/path allowlist, request budget and re-login sequence. Never add a remote write route.

Use only synthetic fixtures. Do not commit or attach credentials, tokens, cookies, raw responses, HAR files, SQLite databases, screenshots or exports that contain real people.

Printing Press changes must keep this command green:

```bash
cli-printing-press generate \
  --spec ./printing-press/myyolo-pp-spec.yaml \
  --spec-source browser-sniffed \
  --transport standard \
  --dry-run
```
