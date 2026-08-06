# Vault Integration (AppRole)

Vault config is split into:
1. Global Vault connections
2. Secret mappings in `ciwi-project.yaml` for step environments or version auto-bump credentials

## 1) Add Vault connection

From `/vault` page, configure:
- `name`
- `url`
- `approle_mount`
- `role_id`
- `secret_id_env`

Use **Test** to validate AppRole login.

## 2) Configure step-local mappings in YAML

```yaml
steps:
  - run: github-release ... --security-token "$GITHUB_SECRET"
    vault:
      connection: home-vault
      secrets:
        - name: github_secret
          mount: kv
          path: gh
          key: token
    env:
      GITHUB_SECRET: "{{ secret.github_secret }}"
```

When omitted, `mount` and `kv_version` use the connection defaults; remaining
empty values fall back to `kv` and KV v2. Set `kv_version: 1` explicitly for a
KV v1 mount.
Step placeholders are supported only in that same step's `env` values and must
refer to a secret declared in the step's `vault.secrets` list.

## 3) Optional auto-bump credential

Pipeline version auto-bump can resolve its VCS token from Vault:

```yaml
versioning:
  file: VERSION
  auto_bump: patch
  auto_bump_vcs_token: "{{ secret.github-token }}"
  auto_bump_vault:
    connection: home-vault
    secrets:
      - name: github-token
        mount: kv
        path: gh
        key: token
        kv_version: 2
```

Outside these two locations—`steps[].env` and
`versioning.auto_bump_vcs_token`—secret placeholders are rejected.

## Security model

- Secrets resolve on the server when the job is leased, separately for each
  executable step. Dry-run-skipped steps do not trigger Vault reads.
- Plaintext secrets are not persisted in sqlite.
- Jobs with secrets disable shell trace.
- Known secret values are redacted from streamed/final logs.
