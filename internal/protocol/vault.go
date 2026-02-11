package protocol

type VaultConnection struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	AuthMethod     string `json:"auth_method"`
	AppRoleMount   string `json:"approle_mount"`
	RoleID         string `json:"role_id"`
	SecretIDEnv    string `json:"secret_id_env,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	KVDefaultMount string `json:"kv_default_mount,omitempty"`
	KVDefaultVer   int    `json:"kv_default_version,omitempty"`
}

type UpsertVaultConnectionRequest struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	AuthMethod     string `json:"auth_method"`
	AppRoleMount   string `json:"approle_mount"`
	RoleID         string `json:"role_id"`
	SecretIDEnv    string `json:"secret_id_env,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	KVDefaultMount string `json:"kv_default_mount,omitempty"`
	KVDefaultVer   int    `json:"kv_default_version,omitempty"`
}

type TestVaultConnectionRequest struct {
	SecretIDOverride string             `json:"secret_id_override,omitempty"`
	TestSecret       *ProjectSecretSpec `json:"test_secret,omitempty"`
}

type TestVaultConnectionResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type ProjectSecretSpec struct {
	Name      string `json:"name"`
	Mount     string `json:"mount,omitempty"`
	Path      string `json:"path"`
	Key       string `json:"key"`
	KVVersion int    `json:"kv_version,omitempty"`
}

type ProjectVaultSettings struct {
	ProjectID           int64               `json:"project_id"`
	VaultConnectionID   int64               `json:"vault_connection_id,omitempty"`
	VaultConnectionName string              `json:"vault_connection_name,omitempty"`
	Secrets             []ProjectSecretSpec `json:"secrets,omitempty"`
}

type UpdateProjectVaultRequest struct {
	VaultConnectionID   int64               `json:"vault_connection_id"`
	VaultConnectionName string              `json:"vault_connection_name,omitempty"`
	Secrets             []ProjectSecretSpec `json:"secrets"`
}

type TestProjectVaultResponse struct {
	OK      bool              `json:"ok"`
	Details map[string]string `json:"details,omitempty"`
	Message string            `json:"message,omitempty"`
}
