package devpage

import (
	"embed"
	"encoding/json"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/cristalhq/jwt/v5"
)

//go:embed index.html
var content embed.FS

// Handler serves development page and token endpoints.
type Handler struct {
	config config.Config
}

// NewHandler creates a new HTTP handler for the development page.
func NewHandler(cfg config.Config) *Handler {
	return &Handler{
		config: cfg,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/dev/index":
		h.serveIndex(w, r)
	case "/dev/connection_token":
		h.serveConnectionToken(w, r)
	case "/dev/subscription_token":
		h.serveSubscriptionToken(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) serveIndex(w http.ResponseWriter, _ *http.Request) {
	indexHTML, err := content.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}

func (h *Handler) serveConnectionToken(w http.ResponseWriter, _ *http.Request) {
	token, err := h.generateConnectionToken()
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *Handler) serveSubscriptionToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	token, err := h.generateSubscriptionToken(req.Channel)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *Handler) generateConnectionToken() (string, error) {
	now := time.Now()
	claims := &jwt.RegisteredClaims{
		Subject:   "dev-user",
		ExpiresAt: jwt.NewNumericDate(now.Add(20 * time.Second)),
		IssuedAt:  jwt.NewNumericDate(now),
	}

	return h.signToken(claims)
}

func (h *Handler) generateSubscriptionToken(channel string) (string, error) {
	now := time.Now()

	// Create a custom claims struct that embeds RegisteredClaims
	type subscriptionClaims struct {
		jwt.RegisteredClaims
		Channel string `json:"channel"`
	}

	claims := &subscriptionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "dev-user",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Channel: channel,
	}

	return h.signToken(claims)
}

func (h *Handler) signToken(claims interface{}) (string, error) {
	// Use HMAC secret if available
	if h.config.Client.Token.HMACSecretKey == "" {
		return "", jwt.ErrInvalidKey
	}

	signer, err := jwt.NewSignerHS(jwt.HS256, []byte(h.config.Client.Token.HMACSecretKey))
	if err != nil {
		return "", err
	}

	builder := jwt.NewBuilder(signer)
	token, err := builder.Build(claims)
	if err != nil {
		return "", err
	}

	return token.String(), nil
}
