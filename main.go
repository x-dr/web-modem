package main

import (
    "log"
    "net/http"
    "os"

    "github.com/gorilla/mux"
    "github.com/rs/cors"

    "modem-manager/handlers"
)

const (
    defaultPort = "8080"
    apiPrefix   = "/api/v1"
)

func main() {
    // Initialize router
    r := mux.NewRouter()
    api := r.PathPrefix(apiPrefix).Subrouter()

    // Modem routes
    api.HandleFunc("/modems", handlers.ListModems).Methods("GET")
    api.HandleFunc("/modem/send", handlers.SendATCommand).Methods("POST")
    api.HandleFunc("/modem/info", handlers.GetModemInfo).Methods("GET")
    api.HandleFunc("/modem/signal", handlers.GetSignalStrength).Methods("GET")
    
    // SMS routes
    api.HandleFunc("/modem/sms/list", handlers.ListSMS).Methods("GET")
    api.HandleFunc("/modem/sms/send", handlers.SendSMS).Methods("POST")

    // WebSocket and Static files
    r.HandleFunc("/ws", handlers.HandleWebSocket)
    r.PathPrefix("/").Handler(http.FileServer(http.Dir("frontend")))

    // Start server
    port := os.Getenv("PORT")
    if port == "" {
        port = defaultPort
    }

    log.Printf("Server starting on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, cors.AllowAll().Handler(r)))
}
