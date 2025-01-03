// internal/auth/auth.go

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type JWKS struct {
    Keys []JSONWebKey `json:"keys"`
}

type JSONWebKey struct {
    Kty string   `json:"kty"`
    Kid string   `json:"kid"`
    Use string   `json:"use"`
    N   string   `json:"n"`
    E   string   `json:"e"`
    X5c []string `json:"x5c"`
}

type Authenticator struct {
	*oidc.Provider
	oauth2.Config
	managementToken     string
	managementTokenExp  time.Time
	managementTokenLock sync.Mutex
}

func NewAuthenticator() (*Authenticator, error) {
	provider, err := oidc.NewProvider(
		context.Background(),
		"https://"+os.Getenv("AUTH0_DOMAIN")+"/",
	)
	if err != nil {
		return nil, err
	}

	config := oauth2.Config{
		ClientID:     os.Getenv("AUTH0_CLIENT_ID"),
		ClientSecret: os.Getenv("AUTH0_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("AUTH0_CALLBACK_URL"),
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &Authenticator{
		Provider: provider,
		Config:   config,
	}, nil
}

// GetManagementToken gets a new Management API token if needed
func (a *Authenticator) GetManagementToken(ctx context.Context) (string, error) {
	a.managementTokenLock.Lock()
	defer a.managementTokenLock.Unlock()

	// Check if we have a valid token
	if a.managementToken != "" && time.Now().Before(a.managementTokenExp) {
		return a.managementToken, nil
	}

	// Request new token
	payload := map[string]string{
		"client_id":     os.Getenv("AUTH0_CLIENT_ID"),
		"client_secret": os.Getenv("AUTH0_CLIENT_SECRET"),
		"audience":      fmt.Sprintf("https://%s/api/v2/", os.Getenv("AUTH0_DOMAIN")),
		"grant_type":    "client_credentials",
		"scope":         "create:users read:users update:users create:user_tickets",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token request: %w", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("https://%s/oauth/token", os.Getenv("AUTH0_DOMAIN")),
		"application/json",
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get management token: %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	a.managementToken = tokenResp.AccessToken
	a.managementTokenExp = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return a.managementToken, nil
}

// VerifyMFAToken verifies an MFA token with Auth0
func (a *Authenticator) VerifyMFAToken(ctx context.Context, userID, code string) (bool, error) {
	token, err := a.GetManagementToken(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get management token: %w", err)
	}

	payload := map[string]string{
		"client_id": os.Getenv("AUTH0_CLIENT_ID"),
		"code":      code,
		"user_id":   userID,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal verify request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("https://%s/api/v2/guardian/enrollments/verify", os.Getenv("AUTH0_DOMAIN")),
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create verify request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to verify MFA token: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetMFASetupURI gets the MFA enrollment URI from Auth0
func (a *Authenticator) GetMFASetupURI(ctx context.Context, userID string) (string, error) {
	token, err := a.GetManagementToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get management token: %w", err)
	}

	payload := map[string]interface{}{
		"user_id": userID,
		"factor":  "totp",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal enrollment request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("https://%s/api/v2/guardian/factors/totp/enrollment", os.Getenv("AUTH0_DOMAIN")),
		bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create enrollment request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to start MFA enrollment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to start MFA enrollment: %d", resp.StatusCode)
	}

	var enrollResp struct {
		QRCodeURI string `json:"qr_code"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&enrollResp); err != nil {
		return "", fmt.Errorf("failed to decode enrollment response: %w", err)
	}

	return enrollResp.QRCodeURI, nil
}

// // GetPublicKey retrieves the public key from Auth0 for token validation
// func (a *Authenticator) GetPublicKey(token *jwt.Token) (*rsa.PublicKey, error) {
//     // Get the kid from the token header
//     kid, ok := token.Header["kid"].(string)
//     if !ok {
//         return nil, errors.New("kid header not found in token")
//     }
//
//     // Get JWKS URL from the OIDC provider
//     jwksURL := a.Provider.Endpoint().JWKSEndpoint
//     resp, err := http.Get(jwksURL)
//     if err != nil {
//         return nil, err
//     }
//     defer resp.Body.Close()
//
//     var jwks JWKS
//     if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
//         return nil, err
//     }
//
//     // Find the key that matches the kid from the token
//     var key *JSONWebKey
//     for _, k := range jwks.Keys {
//         if k.Kid == kid {
//             key = &k
//             break
//         }
//     }
//
//     if key == nil {
//         return nil, errors.New("unable to find matching key")
//     }
//
//     // Convert key to PEM format
//     cert := "-----BEGIN CERTIFICATE-----\n" + key.X5c[0] + "\n-----END CERTIFICATE-----"
//     block, _ := pem.Decode([]byte(cert))
//     if block == nil {
//         return nil, errors.New("failed to parse certificate PEM")
//     }
//
//     // Parse the certificate
//     x509Cert, err := x509.ParseCertificate(block.Bytes)
//     if err != nil {
//         return nil, err
//     }
//
//     // Get RSA public key
//     publicKey, ok := x509Cert.PublicKey.(*rsa.PublicKey)
//     if !ok {
//         return nil, errors.New("failed to cast public key to RSA")
//     }
//
//     return publicKey, nil
// }
