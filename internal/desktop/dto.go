package desktop

// VaultDTO is a registered vault as the frontend sees it.
type VaultDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CurrentVaultDTO is App.CurrentVault packed into one value for the webview
// (Wails v2 drops the results of methods with more than two). Vault.ID is ""
// when there is no current vault.
type CurrentVaultDTO struct {
	Vault    VaultDTO `json:"vault"`
	Unlocked bool     `json:"unlocked"`
}

// ProjectDTO is a project with its totals. Counts only; never secret names or
// values.
type ProjectDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	EnvCount    int    `json:"envCount"`
	SecretCount int    `json:"secretCount"`
}

// SecretDTO is a secret's metadata. It has no value field, ever; the value
// only leaves through RevealSecret. Times are RFC3339 UTC.
type SecretDTO struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CopyResult tells the UI when the copied value will be cleared; 0 means it
// will not be cleared automatically.
type CopyResult struct {
	ClearInSeconds int `json:"clearInSeconds"`
}

// EnvDTO is an environment with its secret count.
type EnvDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SecretCount int    `json:"secretCount"`
}

// ImportPreview lists the names a .env import would add or touch. Names only.
type ImportPreview struct {
	New       []string `json:"new"`
	Duplicate []string `json:"duplicate"`
}
