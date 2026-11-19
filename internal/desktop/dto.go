package desktop

// VaultDTO is a registered vault as the frontend sees it.
type VaultDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProjectDTO is a project with its totals. Counts only; never secret names or
// values.
type ProjectDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	EnvCount    int    `json:"envCount"`
	SecretCount int    `json:"secretCount"`
}

// EnvDTO is an environment with its secret count.
type EnvDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SecretCount int    `json:"secretCount"`
}
