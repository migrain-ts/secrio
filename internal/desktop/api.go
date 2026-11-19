package desktop

// API is the struct the desktop shell binds to the webview. Wails binds every
// exported method of a bound struct, so API forwards an explicit allowlist of
// App methods and nothing else: Shutdown and any other exported App helper
// stay out of reach of JavaScript.
//
// Adding a method callable from the webview requires adding a forwarder here
// AND adding its name to boundAllowlist in api_test.go. Forwarders keep the
// App signature, except that Wails v2 only supports results of the form
// (), (T), (error) or (T, error); a method returning more must pack them in
// a DTO (see CurrentVault) and be listed in reshaped in api_test.go.
type API struct{ app *App }

// NewAPI wraps a for binding.
func NewAPI(a *App) *API { return &API{app: a} }

func (p *API) ListVaults() ([]VaultDTO, error) { return p.app.ListVaults() }
func (p *API) CreateVault(name, password string) (VaultDTO, error) {
	return p.app.CreateVault(name, password)
}
func (p *API) Unlock(vaultID, password string) error { return p.app.Unlock(vaultID, password) }
func (p *API) Lock() error                           { return p.app.Lock() }
func (p *API) SelectVault(vaultID string) error      { return p.app.SelectVault(vaultID) }

// CurrentVault is the one forwarder whose shape differs from App: Wails v2
// only passes back (value) or (value, error), so the pair is packed.
func (p *API) CurrentVault() (CurrentVaultDTO, error) {
	v, unlocked, err := p.app.CurrentVault()
	if err != nil {
		return CurrentVaultDTO{}, err
	}
	return CurrentVaultDTO{Vault: v, Unlocked: unlocked}, nil
}

func (p *API) ListProjects() ([]ProjectDTO, error) { return p.app.ListProjects() }
func (p *API) CreateProject(name string) (ProjectDTO, error) {
	return p.app.CreateProject(name)
}
func (p *API) DeleteProject(id string) error { return p.app.DeleteProject(id) }
func (p *API) ListEnvironments(projectID string) ([]EnvDTO, error) {
	return p.app.ListEnvironments(projectID)
}
func (p *API) CreateEnvironment(projectID, name string) (EnvDTO, error) {
	return p.app.CreateEnvironment(projectID, name)
}
func (p *API) DeleteEnvironment(id string) error { return p.app.DeleteEnvironment(id) }
func (p *API) ListSecrets(envID string) ([]SecretDTO, error) {
	return p.app.ListSecrets(envID)
}
func (p *API) SaveSecret(envID, name, value, typ string) error {
	return p.app.SaveSecret(envID, name, value, typ)
}
func (p *API) DeleteSecret(envID, name string) error { return p.app.DeleteSecret(envID, name) }
func (p *API) RevealSecret(envID, name string) (string, error) {
	return p.app.RevealSecret(envID, name)
}
func (p *API) CopySecret(envID, name string) (CopyResult, error) {
	return p.app.CopySecret(envID, name)
}
func (p *API) PickEnvFile() (string, error) { return p.app.PickEnvFile() }
func (p *API) PreviewImport(envID, path string) (ImportPreview, error) {
	return p.app.PreviewImport(envID, path)
}
func (p *API) ApplyImport(envID, path, strategy string) error {
	return p.app.ApplyImport(envID, path, strategy)
}
