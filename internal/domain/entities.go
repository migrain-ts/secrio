package domain

import "time"

type Vault struct {
	ID        ID[Vault]
	Name      string
	CreatedAt time.Time
}

type Project struct {
	ID          ID[Project]
	Name        string
	Description string
	CreatedAt   time.Time
}

type Environment struct {
	ID        ID[Environment]
	ProjectID ID[Project]
	Name      string
}

type SecretType string

const (
	TypeText        SecretType = "text"
	TypePassword    SecretType = "password"
	TypeAPIKey      SecretType = "api_key"
	TypeToken       SecretType = "token"
	TypeURL         SecretType = "url"
	TypePrivateKey  SecretType = "private_key"
	TypeCertificate SecretType = "certificate"
	TypeJSON        SecretType = "json"
)

type Secret struct {
	ID          ID[Secret]
	EnvID       ID[Environment]
	Name        SecretName
	Value       []byte
	Description string
	Type        SecretType
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const redactedSecret = "Secret{<redacted>}"

func (Secret) String() string   { return redactedSecret }
func (Secret) GoString() string { return redactedSecret }
