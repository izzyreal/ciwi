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
