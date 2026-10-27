package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"autoauction/internal/auction"
	"autoauction/internal/bid"
	"autoauction/internal/config"
	"autoauction/internal/db"
	"autoauction/internal/favorite"
	"autoauction/internal/httpapi"
	"autoauction/internal/lot"
	"autoauction/internal/notify"
	"autoauction/internal/user"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // ok if .env отсутствует — тогда берутся переменные окружения/дефолты

	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	if err := db.RunMigrations(ctx, pool); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	if err := os.MkdirAll(cfg.UploadsDir, 0o755); err != nil {
		log.Fatalf("uploads dir: %v", err)
	}

	lotsRepo := lot.NewRepo(pool)
	bidsRepo := bid.NewRepo(pool)
	usersRepo := user.NewRepo(pool)
	auctionMgr := auction.NewManager(pool, lotsRepo, bidsRepo, usersRepo, notify.LogNotifier{})
	if err := auctionMgr.Resume(ctx); err != nil {
		log.Fatalf("auction resume: %v", err)
	}

	server := &httpapi.Server{
		JWTSecret:   cfg.JWTSecret,
		JWTTTL:      cfg.JWTTTL,
		CORSOrigins: cfg.CORSOrigins,
		UploadsDir:  cfg.UploadsDir,
		Users:       usersRepo,
		Lots:        lotsRepo,
		Bids:        bidsRepo,
		Favorites:   favorite.NewRepo(pool),
		Auction:     auctionMgr,
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(server),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
