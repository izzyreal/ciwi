# Vault Integration (AppRole)

Vault config is two-layered:
1. Global Vault connection
2. Per-project secret mappings

## 1) Add Vault connection

From `/vault` page, configure:
- `name`
- `url`
- `approle_mount`
- `role_id`
- `secret_id_env`

Use **Test** to validate AppRole login.

## 2) Configure project Vault access

From project page **Vault Access**:
- choose connection
- mappings per line: `name=mount/path#key`
- save and test

## 3) Use in YAML

```yaml
steps:
  - run: github-release ... --security-token "$GITHUB_SECRET"
    env:
      GITHUB_SECRET: "{{ secret.github_secret }}"
```

## Optional declarative mapping in ciwi-project.yaml

```yaml
project:
  vault:
    connection: home-vault
    secrets:
      - name: github-secret
        mount: kv
        path: gh
        key: token
        kv_version: 2
```

## Security model

- Secrets resolve at lease-time.
- Plaintext secrets are not persisted in sqlite.
- Jobs with secrets disable shell trace.
- Known secret values are redacted from streamed/final logs.
