package main

import (
    "net/http"
    "os"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"

    "runner/internal/handlers"
    "runner/internal/logging"
)

func getenv(k, d string) string {
    if v := os.Getenv(k); v != "" { return v }
    return d
}

func main() {
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(15 * time.Minute))

    r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
    r.Post("/pubsub/push", handlers.PubSubPushHandler())

    port := getenv("PORT", "8080")
    logging.Log("info", map[string]any{"msg": "runner (push) listening", "port": port})
    _ = http.ListenAndServe(":"+port, r)
}

