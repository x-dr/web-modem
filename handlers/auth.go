package handlers

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

const (
	headerAPIToken     = "X-API-Token"
	queryToken         = "token"
	bearerPrefix       = "Bearer "
	envAPIToken        = "API_TOKEN"
	envCORSOrigins     = "CORS_ALLOWED_ORIGINS"
	envAllowInsecure   = "ALLOW_INSECURE_NO_AUTH"
)

// apiToken 返回配置的 API 令牌；空字符串表示未配置。
func apiToken() string {
	return strings.TrimSpace(os.Getenv(envAPIToken))
}

// authRequired 在配置了 API_TOKEN 时要求鉴权。
// 若未配置令牌：默认拒绝敏感接口，除非显式 ALLOW_INSECURE_NO_AUTH=true（仅建议本地调试）。
func authRequired() bool {
	if apiToken() != "" {
		return true
	}
	return !strings.EqualFold(os.Getenv(envAllowInsecure), "true")
}

// extractToken 从 Header 或 Query 提取令牌。
// 支持：Authorization: Bearer <token>、X-API-Token、<query token=>（WebSocket 常用）。
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, bearerPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(h, bearerPrefix))
	}
	if t := strings.TrimSpace(r.Header.Get(headerAPIToken)); t != "" {
		return t
	}
	return strings.TrimSpace(r.URL.Query().Get(queryToken))
}

// tokenValid 使用常量时间比较校验令牌。
func tokenValid(got string) bool {
	want := apiToken()
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// AuthenticateRequest 校验请求是否通过鉴权。
func AuthenticateRequest(r *http.Request) bool {
	if !authRequired() {
		return true
	}
	if apiToken() == "" {
		// 未配置令牌且未开启 insecure：拒绝
		return false
	}
	return tokenValid(extractToken(r))
}

// AuthMiddleware 保护 REST API。
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			// CORS 预检由 cors 中间件处理，这里放行
			next.ServeHTTP(w, r)
			return
		}
		if !AuthenticateRequest(r) {
			if apiToken() == "" {
				respondError(w, http.StatusUnauthorized, "server requires API_TOKEN (or set ALLOW_INSECURE_NO_AUTH=true for local dev only)")
				return
			}
			respondError(w, http.StatusUnauthorized, "unauthorized: provide Authorization Bearer token or X-API-Token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AllowedOrigins 解析 CORS_ALLOWED_ORIGINS（逗号分隔）。
// 空：仅同源（不额外放行跨域）；"*"：允许任意来源（不推荐生产）。
func AllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv(envCORSOrigins))
	if raw == "" {
		return nil
	}
	if raw == "*" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// OriginAllowed 判断 Origin 是否允许（WebSocket CheckOrigin 用）。
// 空列表：仅允许无 Origin 或与 Host 一致的同源请求。
func OriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	allowed := AllowedOrigins()
	if len(allowed) == 1 && allowed[0] == "*" {
		return true
	}
	if origin == "" {
		// 非浏览器客户端或同源导航
		return true
	}
	// 同源：Origin 的 host 与请求 Host 一致
	if sameOrigin(origin, r.Host) {
		return true
	}
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}

func sameOrigin(origin, host string) bool {
	// origin 形如 http://localhost:8080
	origin = strings.TrimPrefix(origin, "https://")
	origin = strings.TrimPrefix(origin, "http://")
	// 去掉 path
	if i := strings.IndexByte(origin, '/'); i >= 0 {
		origin = origin[:i]
	}
	return strings.EqualFold(origin, host)
}
