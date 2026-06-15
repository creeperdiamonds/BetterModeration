package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/jmoiron/sqlx"
)

type ctxKey string

const (
	ctxServerID ctxKey = "server_id"
	ctxOrgID    ctxKey = "org_id"
)

// RequireServerKey is middleware that validates the Bearer API key sent by Minecraft plugins.
// On success it injects server_id and org_id into the request context.
// Routes that predate authentication (server redeem) are exempt.
func RequireServerKey(db *sqlx.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			serverID := r.Header.Get("X-Server-Id")

			if auth == "" || serverID == "" {
				writeError(w, http.StatusUnauthorized, "missing Authorization or X-Server-Id header")
				return
			}

			key := strings.TrimPrefix(auth, "Bearer ")
			if key == auth {
				writeError(w, http.StatusUnauthorized, "Authorization must be Bearer token")
				return
			}

			var orgID string
			err := db.QueryRowxContext(r.Context(), `
				SELECT org_id FROM servers
				WHERE id = ? AND api_key_hash = SHA2(?, 256) AND platform != 'DISCORD'`,
				serverID, key,
			).Scan(&orgID)
			if err == sql.ErrNoRows {
				writeError(w, http.StatusUnauthorized, "invalid api key or server id")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "auth check failed")
				return
			}

			ctx := context.WithValue(r.Context(), ctxServerID, serverID)
			ctx = context.WithValue(ctx, ctxOrgID, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func serverIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxServerID).(string)
	return v
}

func orgIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxOrgID).(string)
	return v
}
