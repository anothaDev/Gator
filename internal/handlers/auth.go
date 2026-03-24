package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/anothaDev/gator/internal/models"
)

const (
	authCookieName    = "gator_session"
	sessionLifetime   = 30 * 24 * time.Hour
	minimumPassLength = 8
)

type AuthStore interface {
	HasAdminPassword(ctx context.Context) (bool, error)
	GetAdminPasswordHash(ctx context.Context) (string, error)
	SetAdminPasswordHash(ctx context.Context, hash string) error
	CreateAuthSession(ctx context.Context, tokenHash string, expiresAtUnix int64) error
	GetAuthSessionExpiry(ctx context.Context, tokenHash string) (int64, bool, error)
	DeleteAuthSession(ctx context.Context, tokenHash string) error
	DeleteExpiredAuthSessions(ctx context.Context, nowUnix int64) error
	GetActiveInstance(ctx context.Context) (*models.FirewallInstance, error)
}

type AuthHandler struct {
	store AuthStore
}

func NewAuthHandler(store AuthStore) *AuthHandler {
	return &AuthHandler{store: store}
}

func (h *AuthHandler) Status(c *gin.Context) {
	configured, err := h.store.HasAdminPassword(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth status"})
		return
	}

	authenticated := false
	if configured {
		authenticated, err = h.isAuthenticated(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth session"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"configured":    configured,
		"authenticated": authenticated,
	})
}

func (h *AuthHandler) Bootstrap(c *gin.Context) {
	configured, err := h.store.HasAdminPassword(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth status"})
		return
	}
	if configured {
		c.JSON(http.StatusConflict, gin.H{"error": "admin password already configured"})
		return
	}

	active, err := h.store.GetActiveInstance(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read setup state"})
		return
	}
	if active == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "complete firewall setup before enabling authentication"})
		return
	}

	password, ok := readPassword(c)
	if !ok {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure password"})
		return
	}

	if err := h.store.SetAdminPasswordHash(c.Request.Context(), string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save admin password"})
		return
	}

	if err := h.startSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create auth session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "bootstrapped"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	configured, err := h.store.HasAdminPassword(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth status"})
		return
	}
	if !configured {
		c.JSON(http.StatusConflict, gin.H{"error": "admin password not configured yet"})
		return
	}

	password, ok := readPassword(c)
	if !ok {
		return
	}

	hash, err := h.store.GetAdminPasswordHash(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read admin password"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		return
	}

	if err := h.startSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create auth session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "authenticated"})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token, err := c.Cookie(authCookieName)
	if err == nil && strings.TrimSpace(token) != "" {
		_ = h.store.DeleteAuthSession(c.Request.Context(), hashToken(token))
	}

	clearAuthCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "logged_out"})
}

func (h *AuthHandler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if isStaticPublicPath(path) || path == "/health" {
			c.Next()
			return
		}

		configured, err := h.store.HasAdminPassword(c.Request.Context())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to read auth state"})
			return
		}

		if !configured {
			if isBootstrapPublicPath(path) {
				c.Next()
				return
			}
			h.reject(c, "/setup", "auth setup required")
			return
		}

		if isLoginPublicPath(path) {
			c.Next()
			return
		}

		authenticated, err := h.isAuthenticated(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to validate auth session"})
			return
		}
		if authenticated {
			c.Next()
			return
		}

		h.reject(c, "/login", "authentication required")
	}
}

func (h *AuthHandler) reject(c *gin.Context, redirectTarget, message string) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectTarget)
	c.Abort()
}

func (h *AuthHandler) isAuthenticated(c *gin.Context) (bool, error) {
	token, err := c.Cookie(authCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(token) == "" {
		return false, nil
	}

	now := time.Now().UTC().Unix()
	if err := h.store.DeleteExpiredAuthSessions(c.Request.Context(), now); err != nil {
		return false, err
	}

	expiresAtUnix, ok, err := h.store.GetAuthSessionExpiry(c.Request.Context(), hashToken(token))
	if err != nil {
		return false, err
	}
	if !ok || expiresAtUnix <= now {
		clearAuthCookie(c)
		return false, nil
	}

	return true, nil
}

func (h *AuthHandler) startSession(c *gin.Context) error {
	rawToken, err := randomToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().UTC().Add(sessionLifetime)
	if err := h.store.CreateAuthSession(c.Request.Context(), hashToken(rawToken), expiresAt.Unix()); err != nil {
		return err
	}

	setAuthCookie(c, rawToken, expiresAt)
	return nil
}

func readPassword(c *gin.Context) (string, bool) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return "", false
	}

	password := strings.TrimSpace(req.Password)
	if len(password) < minimumPassLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("password must be at least %d characters", minimumPassLength)})
		return "", false
	}

	return password, true
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}

func setAuthCookie(c *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(c),
		Expires:  expiresAt,
		MaxAge:   int(sessionLifetime.Seconds()),
	})
}

func clearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(c),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func requestIsSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func isStaticPublicPath(path string) bool {
	if strings.HasPrefix(path, "/assets/") {
		return true
	}

	publicFiles := map[string]bool{
		"/gator-logo.svg": true,
		"/favicon.ico":    true,
	}

	return publicFiles[path]
}

func isBootstrapPublicPath(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return path == "/setup" || path == "/login" || path == "/"
	}

	publicAPIs := map[string]bool{
		"/api/setup/status":             true,
		"/api/setup/save":               true,
		"/api/setup/test":               true,
		"/api/opnsense/test-connection": true,
		"/api/pfsense/test-connection":  true,
		"/api/auth/status":              true,
		"/api/auth/bootstrap":           true,
		"/api/auth/login":               true,
		"/api/auth/logout":              true,
	}

	return publicAPIs[path]
}

func isLoginPublicPath(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return path == "/login"
	}

	publicAPIs := map[string]bool{
		"/api/setup/status": true,
		"/api/auth/status":  true,
		"/api/auth/login":   true,
		"/api/auth/logout":  true,
	}

	return publicAPIs[path]
}
