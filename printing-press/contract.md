# Printing Press contract

The spec in `myyolo-pp-spec.yaml` records the two observed JSON routes used by
the CLI. It contains no credentials, cookies, tokens, member records, captured
responses or HAR files.

Printing Press is used here as a reproducible contract and generator check, not
as the runtime security boundary. Version 4.22.1 classifies the two `POST`
operations syntactically as create-style commands and cannot express all of
these runtime invariants in the generated client:

- `POST /Home/GetListData` is semantically read-only;
- only the two exact host/method/path combinations may leave the process;
- passwords belong in the operating-system keyring;
- a sync may make at most three requests;
- an expired session causes exactly one re-login;
- a schema change must stop without re-authentication;
- all other myYOLO routes are denied.

The hand-written packages under `internal/transport` and `internal/secrets`
therefore remain authoritative. A generated tree may be used for comparison,
but is deliberately not committed or executed against a live account.

## Reproduce

```bash
cli-printing-press generate \
  --spec ./printing-press/myyolo-pp-spec.yaml \
  --spec-source browser-sniffed \
  --transport standard \
  --dry-run
```

The dry run must parse the spec without credentials or live traffic.
