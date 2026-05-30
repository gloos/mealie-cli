package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// About is the public server information from GET /api/app/about. It needs no
// authentication and is used by `mealie doctor` and `mealie version` to verify
// connectivity and the server version.
type About struct {
	Version          string `json:"version"`
	Production       bool   `json:"production"`
	Demo             bool   `json:"demoStatus"`
	AllowSignup      bool   `json:"allowSignup"`
	DefaultGroupSlug string `json:"defaultGroupSlug,omitempty"`
	EnableOIDC       bool   `json:"enableOidc"`
}

// About fetches the public server information.
func (c *Client) About(ctx context.Context) (*About, error) {
	var a About
	if err := c.do(ctx, "GET", "/api/app/about", nil, nil, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// User is the authenticated user from GET /api/users/self.
type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"fullName,omitempty"`
	Email     string `json:"email,omitempty"`
	Admin     bool   `json:"admin"`
	Group     string `json:"group,omitempty"`
	Household string `json:"household,omitempty"`
}

// Whoami returns the user the current token authenticates as. A 401 indicates an
// absent or invalid token.
func (c *Client) Whoami(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, "GET", "/api/users/self", nil, nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Login authenticates with a username and password against POST /api/auth/token
// and, when tokenName is non-empty, mints a long-lived API token named tokenName
// via POST /api/users/api-tokens. The returned string is the long-lived token
// (or the short-lived access token if tokenName is empty). The client's own
// token is not used or modified.
func (c *Client) Login(ctx context.Context, username, password, tokenName string) (string, error) {
	form := url.Values{
		"username": {username},
		"password": {password},
	}
	var session struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.request(ctx, "", "POST", "/api/auth/token", nil, "application/x-www-form-urlencoded", []byte(form.Encode()), &session); err != nil {
		return "", err
	}
	if session.AccessToken == "" {
		return "", fmt.Errorf("login succeeded but no access token was returned")
	}
	if tokenName == "" {
		return session.AccessToken, nil
	}

	body, _ := json.Marshal(map[string]string{"name": tokenName})
	var minted struct {
		Token string `json:"token"`
	}
	if err := c.request(ctx, session.AccessToken, "POST", "/api/users/api-tokens", nil, "application/json", body, &minted); err != nil {
		return "", fmt.Errorf("mint API token: %w", err)
	}
	if minted.Token == "" {
		return "", fmt.Errorf("API token creation returned an empty token")
	}
	return minted.Token, nil
}
