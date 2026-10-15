package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/oidcauth"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/postgres"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
	"go.uber.org/fx"
)

type config struct {
	databaseURL    string
	httpAddr       string
	migrationsDir  string
	oidcIssuer     string
	walletClientID string
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
			newTokenVerifier,
			newAuthenticator,
			application.NewOpenWalletService,
			application.NewPlaceBetService,
			httpapi.NewWalletHandler,
			httpapi.NewWagerTransactionHandler,
			httpapi.NewHandler,
		),
		fx.Invoke(serve),
	).Run()
}

func loadConfig() (config, error) {
	if err := loadProjectEnv(); err != nil {
		return config{}, err
	}
	databaseURL, err := requiredEnv("DATABASE_URL")
	if err != nil {
		return config{}, err
	}
	httpAddr, err := requiredEnv("HTTP_ADDR")
	if err != nil {
		return config{}, err
	}
	migrationsDir, err := requiredEnv("MIGRATIONS_DIR")
	if err != nil {
		return config{}, err
	}
	oidcIssuer, err := requiredEnv("OIDC_ISSUER")
	if err != nil {
		return config{}, err
	}
	walletClientID, err := requiredEnv("WALLET_CLIENT_ID")
	if err != nil {
		return config{}, err
	}
	return config{
		databaseURL:    databaseURL,
		httpAddr:       httpAddr,
		migrationsDir:  migrationsDir,
		oidcIssuer:     oidcIssuer,
		walletClientID: walletClientID,
	}, nil
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
	return uuid.NewString
}

func newClock() func() time.Time {
	return time.Now
}

func newTokenVerifier(cfg config) *oidcauth.Verifier {
	return oidcauth.NewVerifier(cfg.oidcIssuer)
}

func newAuthenticator(tokens *oidcauth.Verifier, cfg config) *httpapi.Authenticator {
	return httpapi.NewAuthenticator(tokens, cfg.walletClientID)
}

func serve(
	lc fx.Lifecycle,
	authenticator *httpapi.Authenticator,
	handler http.Handler,
	bets *application.PlaceBetService,
	cfg config,
) {
	server := &http.Server{Addr: cfg.httpAddr, Handler: authenticator.Wrap(handler)}
	workerCtx, stopWorker := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			listener, err := net.Listen("tcp", cfg.httpAddr)
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			go pollPendingReferences(workerCtx, bets)
			go func() {
				if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
					log.Printf("http server: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			stopWorker()
			return server.Shutdown(ctx)
		},
	})
}

func pollPendingReferences(ctx context.Context, bets *application.PlaceBetService) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := bets.AdvanceDueReferences(ctx); err != nil && ctx.Err() == nil {
				log.Printf("pending references: %v", err)
			}
		}
	}
}

func loadProjectEnv() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("working directory: %w", err)
	}
	for {
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			return loadEnvFile(envPath)
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func loadEnvFile(path string) error {
	err := godotenv.Load(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}
