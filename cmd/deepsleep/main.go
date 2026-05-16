package main

import (
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"deepsleep.local/deepsleep0b/internal/api"
	"deepsleep.local/deepsleep0b/internal/appconfig"
	"deepsleep.local/deepsleep0b/internal/generator"
	"deepsleep.local/deepsleep0b/internal/slop"
)

func main() {
	addr := flag.String("addr", defaultAddr(), "HTTP listen address")
	slopPath := flag.String("slop", envDefault("SLOP_FILE", "data/slop.json"), "phrase JSON path")
	indexPath := flag.String("index", envDefault("INDEX_FILE", "web/index.html"), "frontend HTML path")
	configPath := flag.String("config", envDefault("CONFIG_FILE", "config.json"), "config JSON path")
	flag.Parse()

	appConfig, err := appconfig.LoadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	entries, err := slop.LoadFile(*slopPath)
	if err != nil {
		log.Fatal(err)
	}
	indexHTML, err := os.ReadFile(*indexPath)
	if err != nil {
		log.Fatalf("read %s: %v", *indexPath, err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	handler := api.NewServer(api.Config{
		Generator: generator.New(entries, rng),
		IndexHTML: indexHTML,
		Domain:    appConfig.Domain,
	})

	log.Printf("deepsleep listening on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}

func defaultAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		return ":8080"
	}
	return ":" + port
}

func envDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
