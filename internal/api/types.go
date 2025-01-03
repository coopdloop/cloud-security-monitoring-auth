// internal/api/types.go
package api

type Claims struct {
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	Picture       string   `json:"picture,omitempty"`
	Nickname      string   `json:"nickname"`
	Sub           string   `json:"sub"`
	UpdatedAt     string   `json:"updated_at"`
	Roles         []string `json:"https://your-domain/roles"`
	Permissions   []string `json:"https://your-domain/permissions"`
	MFAEnabled    bool     `json:"mfa_enabled"`
}
