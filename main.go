package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"web-modem/handlers"
)

const (
	defaultPort = "8080"
	apiPrefix   = "/api/v1"
)

func main() {
	if err := validateSecurityConfig(); err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()
	api := r.PathPrefix(apiPrefix).Subrouter()
	api.Use(handlers.AuthMiddleware)

	// 调制解调器路由
	api.HandleFunc("/modems", handlers.ListModems).Methods("GET")
	api.HandleFunc("/modem/at", handlers.SendATCommand).Methods("POST")
	api.HandleFunc("/modem/info", handlers.GetModemInfo).Methods("GET")
	api.HandleFunc("/modem/signal", handlers.GetSignalStrength).Methods("GET")

	// 短信读写路由
	api.HandleFunc("/modem/sms/list", handlers.ListSMS).Methods("GET")
	api.HandleFunc("/modem/sms/send", handlers.SendSMS).Methods("POST")
	api.HandleFunc("/modem/sms/delete", handlers.DeleteSMS).Methods("POST")

	// WebSocket（内部校验 Origin + Token）
	r.HandleFunc("/ws", handlers.HandleWebSocket)

	// 静态文件服务（前端页面无需鉴权；API 调用时由前端带 Token）
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("frontend")))

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	handler := buildCORS(r)

	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func validateSecurityConfig() error {
	token := strings.TrimSpace(os.Getenv("API_TOKEN"))
	insecure := strings.EqualFold(os.Getenv("ALLOW_INSECURE_NO_AUTH"), "true")
	if token == "" && insecure {
		log.Println("WARNING: ALLOW_INSECURE_NO_AUTH=true 且未设置 API_TOKEN，API/WebSocket 无鉴权，切勿暴露到公网")
		return nil
	}
	if token == "" {
		log.Println("WARNING: 未设置 API_TOKEN。生产环境请设置 API_TOKEN；本地调试可设 ALLOW_INSECURE_NO_AUTH=true")
	} else {
		log.Println("API 鉴权已启用（Authorization: Bearer / X-API-Token）")
	}
	origins := handlers.AllowedOrigins()
	if len(origins) == 1 && origins[0] == "*" {
		log.Println("WARNING: CORS_ALLOWED_ORIGINS=* 允许任意跨域来源")
	} else if len(origins) == 0 {
		log.Println("CORS: 仅同源（可通过 CORS_ALLOWED_ORIGINS 追加来源）")
	} else {
		log.Printf("CORS 允许来源: %s", strings.Join(origins, ", "))
	}
	return nil
}

func buildCORS(h http.Handler) http.Handler {
	origins := handlers.AllowedOrigins()
	if len(origins) == 1 && origins[0] == "*" {
		return cors.New(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
			AllowedHeaders:   []string{"Content-Type", "Authorization", "X-API-Token"},
			AllowCredentials: false,
		}).Handler(h)
	}
	// 无额外来源时仍启用 cors 中间件以便处理自定义 Header 的预检；
	// AllowedOrigins 为空表示不反射任意 Origin（同源请求不依赖 CORS）。
	opts := cors.Options{
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-API-Token"},
		AllowCredentials: true,
	}
	if len(origins) > 0 {
		opts.AllowedOrigins = origins
	} else {
		// 明确不使用 AllowAll；仅允许请求自带的同源场景
		opts.AllowOriginFunc = func(origin string) bool {
			// 同源由浏览器直接放行；跨域默认拒绝
			return false
		}
	}
	return cors.New(opts).Handler(h)
}
