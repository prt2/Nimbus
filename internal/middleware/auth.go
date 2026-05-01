package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte("nimbus-super-secret-key-change-in-production")

type contextKey string

const TenantKey contextKey = "tenant"

func init() {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		jwtSecret = []byte(secret)
	}
}

func SetJWTSecret(secret string) {
	if secret != "" {
		jwtSecret = []byte(secret)
	}
}

func GenerateToken(tenantID, tenantName string) (string, error) {
	claims := jwt.MapClaims{
		"tenant_id":   tenantID,
		"tenant_name": tenantName,
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check, token endpoint, and metrics
		if r.URL.Path == "/health" || r.URL.Path == "/auth/token" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		claims := token.Claims.(jwt.MapClaims)
		ctx := context.WithValue(r.Context(), TenantKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ClaimsFromContext(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value(TenantKey).(jwt.MapClaims)
	return claims, ok
}

func TenantIDFromContext(ctx context.Context) (string, error) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("missing auth claims")
	}

	tenantID, ok := claims["tenant_id"].(string)
	if !ok || tenantID == "" {
		return "", fmt.Errorf("missing tenant id in token")
	}

	return tenantID, nil
}

func IsAdmin(ctx context.Context) bool {
	tenantID, err := TenantIDFromContext(ctx)
	return err == nil && tenantID == "admin"
}
