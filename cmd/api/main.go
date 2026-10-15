package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/postgres"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
	"go.uber.org/fx"
)

type config struct {
	databaseURL   string
	httpAddr      string
	migrationsDir string
}

func main() {
	fx.New(
		fx.Provide(
			loadConfig,
			newPool,
			fx.Annotate(
				newStore,
				fx.As(new(application.WalletOpeningStore)),
				fx.As(new(application.BetStore)),
			),
			newIdentifier,
			newClock,
			application.NewOpenWalletService,
			application.NewPlaceBetService,
			httpapi.NewWalletHandler,
			httpapi.NewWagerTransactionHandler,
			httpapi.NewHandler,
		),
		fx.Invoke(serve),
	).Run()
}

func loadConfig() config {
	return config{
		databaseURL:   env("DATABASE_URL", "postgres://jungle:jungle@127.0.0.1:55432/jungle?sslmode=disable"),
		httpAddr:      env("HTTP_ADDR", ":8080"),
		migrationsDir: env("MIGRATIONS_DIR", "migrations"),
	}
}

func newPool(lc fx.Lifecycle, cfg config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var pool *pgxpool.Pool
	var err error
	for {
		pool, err = pgxpool.New(ctx, cfg.databaseURL)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err == nil {
			break
		}
		if pool != nil {
			pool.Close()
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("connect database: %w", err)
		}
		time.Sleep(time.Second)
	}

	if err := postgres.ApplyMigrations(ctx, pool, cfg.migrationsDir); err != nil {
		pool.Close()
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			pool.Close()
			return nil
		},
	})
	return pool, nil
}

func newStore(pool *pgxpool.Pool) *postgres.WalletOpeningStore {
	return postgres.NewWalletOpeningStore(pool)
}

func newIdentifier() func() string {
	return func() string {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			panic(fmt.Errorf("generate id: %w", err))
		}
		raw[6] = (raw[6] & 0x0f) | 0x40
		raw[8] = (raw[8] & 0x3f) | 0x80
		encoded := hex.EncodeToString(raw[:])
		return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
	}
}

func newClock() func() time.Time {
	return time.Now
}

func serve(lc fx.Lifecycle, handler http.Handler, cfg config) {
	server := &http.Server{Addr: cfg.httpAddr, Handler: handler}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			listener, err := net.Listen("tcp", cfg.httpAddr)
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			go func() {
				if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
					log.Printf("http server: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
}

func env(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
