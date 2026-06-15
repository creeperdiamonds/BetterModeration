package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	discordAuthURL  = "https://discord.com/api/oauth2/authorize"
	discordTokenURL = "https://discord.com/api/oauth2/token"
	discordUserURL  = "https://discord.com/api/users/@me"
)

// Session holds the data embedded in a signed session token.
type Session struct {
	DiscordID string    `json:"did"`
	Username  string    `json:"usr"`
	IssuedAt  time.Time `json:"iat"`
}

// Manager handles Discord OAuth2 and session token issuance.
type Manager struct {
	clientID     string
	clientSecret string
	redirectURL  string
	signingKey   []byte
}

func NewManager(clientID, clientSecret, redirectURL, signingKey string) *Manager {
	return &Manager{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		signingKey:   []byte(signingKey),
	}
}

// NewManagerFromEnv reads DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET,
// DISCORD_REDIRECT_URL, and SESSION_SIGNING_KEY from the environment.
func NewManagerFromEnv() *Manager {
	return NewManager(
		os.Getenv("DISCORD_CLIENT_ID"),
		os.Getenv("DISCORD_CLIENT_SECRET"),
		os.Getenv("DISCORD_REDIRECT_URL"),
		os.Getenv("SESSION_SIGNING_KEY"),
	)
}

// AuthCodeURL returns the Discord OAuth2 redirect URL with a random state.
func (m *Manager) AuthCodeURL() (redirectURL, state string, err error) {
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return
	}
	state = hex.EncodeToString(b)

	params := url.Values{
		"client_id":     {m.clientID},
		"redirect_uri":  {m.redirectURL},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
	}
	redirectURL = discordAuthURL + "?" + params.Encode()
	return
}

// Exchange swaps an OAuth2 code for a Discord user and issues a session token.
func (m *Manager) Exchange(ctx context.Context, code string) (token string, sess *Session, err error) {
	// Exchange code for access token
	form := url.Values{
		"client_id":     {m.clientID},
		"client_secret": {m.clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {m.redirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordTokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return
	}
	if tokenResp.AccessToken == "" {
		err = fmt.Errorf("no access_token in Discord response")
		return
	}

	// Fetch user info
	userReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, discordUserURL, nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		return
	}
	defer userResp.Body.Close()

	body, _ := io.ReadAll(userResp.Body)
	var user struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err = json.Unmarshal(body, &user); err != nil {
		return
	}

	sess = &Session{
		DiscordID: user.ID,
		Username:  user.Username,
		IssuedAt:  time.Now().UTC(),
	}
	token, err = m.sign(sess)
	return
}

// Verify parses and verifies a session token, returning nil if invalid or expired.
func (m *Manager) Verify(token string) *Session {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil
	}
	sig, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	// Verify HMAC
	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return nil
	}
	var sess Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return nil
	}
	// Sessions expire after 7 days
	if time.Since(sess.IssuedAt) > 7*24*time.Hour {
		return nil
	}
	return &sess
}

func (m *Manager) sign(sess *Session) (string, error) {
	payload, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(payload) + "." + sig, nil
}
