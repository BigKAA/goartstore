// Пакет keycloak — HTTP-клиент к Keycloak Admin REST API.
// models.go — модели данных Keycloak.
package keycloak

import "time"

// TokenResponse — ответ на запрос токена через Client Credentials flow.
type TokenResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // G117: структура токена OAuth2
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// KeycloakUser — пользователь в Keycloak.
type KeycloakUser struct { //nolint:revive // stuttering допустим — внешний API Keycloak
	ID            string `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
	Enabled       bool   `json:"enabled"`
	CreatedAt     int64  `json:"createdTimestamp"`
	EmailVerified bool   `json:"emailVerified"`
}

// CreatedAtTime возвращает CreatedAt как time.Time.
// Keycloak хранит timestamp в миллисекундах.
func (u *KeycloakUser) CreatedAtTime() time.Time {
	return time.UnixMilli(u.CreatedAt)
}

// KeycloakGroup — группа в Keycloak.
type KeycloakGroup struct { //nolint:revive // stuttering допустим — внешний API Keycloak
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// KeycloakClient — клиент (application) в Keycloak.
// Используется для Service Accounts (Client Credentials grant).
type KeycloakClient struct { //nolint:revive // stuttering допустим — внешний API Keycloak
	ID                      string `json:"id"`
	ClientID                string `json:"clientId"`
	Name                    string `json:"name,omitempty"`
	Description             string `json:"description,omitempty"`
	Enabled                 bool   `json:"enabled"`
	ServiceAccountsEnabled  bool   `json:"serviceAccountsEnabled"`
	ClientAuthenticatorType string `json:"clientAuthenticatorType,omitempty"`
	// DefaultClientScopes — scopes клиента.
	DefaultClientScopes []string `json:"defaultClientScopes,omitempty"`
	// Attributes — атрибуты клиента.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// KeycloakClientSecret — секрет клиента.
type KeycloakClientSecret struct { //nolint:revive // stuttering допустим — внешний API Keycloak
	Type  string `json:"type"`
	Value string `json:"value"`
}

// RealmRepresentation — краткая информация о realm.
type RealmRepresentation struct {
	Realm   string `json:"realm"`
	Enabled bool   `json:"enabled"`
}

// clientCreateRequest — запрос на создание клиента в Keycloak.
// Используется внутренне; поля соответствуют Keycloak Admin REST API.
type clientCreateRequest struct {
	ClientID                  string            `json:"clientId"`
	Name                      string            `json:"name,omitempty"`
	Description               string            `json:"description,omitempty"`
	Enabled                   bool              `json:"enabled"`
	ServiceAccountsEnabled    bool              `json:"serviceAccountsEnabled"`
	ClientAuthenticatorType   string            `json:"clientAuthenticatorType"`
	DirectAccessGrantsEnabled bool              `json:"directAccessGrantsEnabled"`
	StandardFlowEnabled       bool              `json:"standardFlowEnabled"`
	PublicClient              bool              `json:"publicClient"`
	DefaultClientScopes       []string          `json:"defaultClientScopes,omitempty"`
	Attributes                map[string]string `json:"attributes,omitempty"`
}
