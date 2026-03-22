package api

import (
	"net/http"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/openilink/openilink-hub/internal/auth"
	"github.com/openilink/openilink-hub/internal/bot"
	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/relay"
)

type Server struct {
	DB           *database.DB
	WebAuthn     *webauthn.WebAuthn
	SessionStore *auth.SessionStore
	BotManager   *bot.Manager
	Hub          *relay.Hub
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// --- Public auth ---
	mux.HandleFunc("POST /api/auth/register", s.handlePasswordRegister)
	mux.HandleFunc("POST /api/auth/login", s.handlePasswordLogin)
	mux.HandleFunc("POST /api/auth/passkey/register/begin", s.handleRegisterBegin)
	mux.HandleFunc("POST /api/auth/passkey/register/finish", s.handleRegisterFinish)
	mux.HandleFunc("POST /api/auth/passkey/login/begin", s.handleLoginBegin)
	mux.HandleFunc("POST /api/auth/passkey/login/finish", s.handleLoginFinish)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// --- WebSocket (sub-level auth via api_key) ---
	mux.HandleFunc("GET /api/ws", s.handleWebSocket)

	// --- Protected routes ---
	protected := http.NewServeMux()

	// Profile
	protected.HandleFunc("GET /api/auth/me", s.handleMe)
	protected.HandleFunc("PUT /api/auth/profile", s.handleUpdateProfile)
	protected.HandleFunc("PUT /api/auth/password", s.handleChangePassword)

	// Bots
	protected.HandleFunc("GET /api/bots", s.handleListBots)
	protected.HandleFunc("POST /api/bots/bind/start", s.handleBindStart)
	protected.HandleFunc("GET /api/bots/bind/status/{sessionID}", s.handleBindStatus)
	protected.HandleFunc("POST /api/bots/{id}/reconnect", s.handleReconnect)
	protected.HandleFunc("DELETE /api/bots/{id}", s.handleDeleteBot)

	// Sub-levels
	protected.HandleFunc("GET /api/sublevels", s.handleListSublevels)
	protected.HandleFunc("POST /api/sublevels", s.handleCreateSublevel)
	protected.HandleFunc("PUT /api/sublevels/{id}", s.handleUpdateSublevel)
	protected.HandleFunc("DELETE /api/sublevels/{id}", s.handleDeleteSublevel)
	protected.HandleFunc("POST /api/sublevels/{id}/rotate-key", s.handleRotateKey)

	// Bot stats & contacts
	protected.HandleFunc("GET /api/stats", s.handleStats)
	protected.HandleFunc("GET /api/bots/{id}/contacts", s.handleBotContacts)
	protected.HandleFunc("PUT /api/bots/{id}/name", s.handleUpdateBotName)

	// Messages
	protected.HandleFunc("GET /api/messages", s.handleListMessages)

	// --- Admin: user management ---
	protected.HandleFunc("GET /api/users", s.requireAdmin(s.handleListUsers))
	protected.HandleFunc("POST /api/users", s.requireAdmin(s.handleCreateUser))
	protected.HandleFunc("PUT /api/users/{id}/role", s.requireAdmin(s.handleUpdateUserRole))
	protected.HandleFunc("PUT /api/users/{id}/status", s.requireAdmin(s.handleUpdateUserStatus))
	protected.HandleFunc("PUT /api/users/{id}/password", s.requireAdmin(s.handleResetUserPassword))
	protected.HandleFunc("DELETE /api/users/{id}", s.requireAdmin(s.handleDeleteUser))

	mux.Handle("/api/", auth.Middleware(s.DB)(protected))

	return mux
}
