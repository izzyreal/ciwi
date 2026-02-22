# Vault Integration (AppRole)

Vault config is split into:
1. Global Vault connections
2. Step-local secret mappings in `ciwi-project.yaml`

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

Only `steps[].env` supports `{{ secret.<name> }}` placeholders.
Secret placeholders outside step env are rejected.

## Security model

- Secrets resolve at lease-time.
- Plaintext secrets are not persisted in sqlite.
- Jobs with secrets disable shell trace.
- Known secret values are redacted from streamed/final logs.
