package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"creeperdiamonds.xyz/bettermoderation/internal/api"
	"creeperdiamonds.xyz/bettermoderation/internal/appeals"
	"creeperdiamonds.xyz/bettermoderation/internal/auth"
	"creeperdiamonds.xyz/bettermoderation/internal/automod"
	"creeperdiamonds.xyz/bettermoderation/internal/bot"
	"creeperdiamonds.xyz/bettermoderation/internal/cache"
	"creeperdiamonds.xyz/bettermoderation/internal/db"
	"creeperdiamonds.xyz/bettermoderation/internal/expiry"
	"creeperdiamonds.xyz/bettermoderation/internal/linking"
	"creeperdiamonds.xyz/bettermoderation/internal/permission"
	"creeperdiamonds.xyz/bettermoderation/internal/profile"
	"creeperdiamonds.xyz/bettermoderation/internal/reports"
	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
	"creeperdiamonds.xyz/bettermoderation/internal/warn"
	"creeperdiamonds.xyz/bettermoderation/internal/webhook"
)

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func main() {
	databaseURL := getEnv("DATABASE_URL", "")
	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")
	discordToken := getEnv("DISCORD_TOKEN", "")
	port := getEnv("PORT", "8080")

	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	if discordToken == "" {
		log.Fatal("DISCORD_TOKEN environment variable is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Initialize MariaDB/MySQL
	database, err := db.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect to mariadb: %v", err)
	}
	defer database.Close()
	log.Println("connected to MariaDB")

	// Initialize Redis
	redisCache, err := cache.New(redisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer redisCache.Close()
	if err := redisCache.Ping(context.Background()); err != nil {
		log.Fatalf("redis ping failed: %v", err)
	}
	log.Println("connected to Redis")

	// Initialize services
	warnSvc := warn.NewService(database.Conn)
	profileSvc := profile.NewService(database.Conn)
	linkingSvc := linking.NewService(database.Conn)
	appealsSvc := appeals.NewService(database.Conn)
	reportsSvc := reports.NewService(database.Conn)
	webhookDisp := webhook.NewDispatcher(database.Conn)
	permLoader := permission.NewLoader(database.Conn, redisCache.Client())
	eventBus := bmsync.NewEventBus(redisCache.Client())
	autoModEngine := automod.NewEngine(database.Conn, redisCache.Client())

	// Initialize Discord bot
	discordBot, err := bot.New(discordToken, warnSvc, profileSvc, permLoader, eventBus, autoModEngine, appealsSvc, reportsSvc, database.Conn)
	if err != nil {
		log.Fatalf("failed to create discord bot: %v", err)
	}
	if err := discordBot.Start(); err != nil {
		log.Fatalf("failed to start discord bot: %v", err)
	}
	defer discordBot.Stop()
	log.Println("Discord bot started")

	// Start expiry worker — checks for expired bans/mutes every 30 seconds
	expiryWorker := expiry.NewWorker(database.Conn, discordBot.Session, eventBus, 30*time.Second)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go expiryWorker.Run(workerCtx)
	log.Println("expiry worker started")

	// Initialize Discord OAuth2 manager
	oauthMgr := auth.NewManagerFromEnv()

	// Set up HTTP router
	router := api.NewRouter(profileSvc, warnSvc, linkingSvc, appealsSvc, reportsSvc, webhookDisp, eventBus, oauthMgr, autoModEngine, permLoader, redisCache.Client(), database.Conn)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE streams need no write timeout
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server in background
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutdown signal received, shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server forced to shutdown: %v", err)
	}

	log.Println("server exited cleanly")
}
