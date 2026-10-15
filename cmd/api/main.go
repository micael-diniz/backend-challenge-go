package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/sqs"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
	"go.uber.org/fx"
)

var (
	errPostgresNotReady        = errors.New("postgres is not ready")
	errSQSNotReady             = errors.New("sqs is not ready")
	errSQSReadinessUnavailable = errors.New("sqs readiness is unavailable")
)

type config struct {
	databaseURL    string
	httpAddr       string
	migrationsDir  string
	oidcIssuer     string
	walletClientID string
	sqsEndpoint    string
}

type messaging struct {
	wagers application.MessageQueue
	events application.EventPublisher
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	fx.New(
		fx.Provide(
			loadConfig,
			newPool,
			fx.Annotate(
				newStore,
				fx.As(new(application.WalletOpeningStore)),
				fx.As(new(application.BetStore)),
				fx.As(new(application.ReadStore)),
				fx.As(new(application.InboxStore)),
				fx.As(new(application.OutboxStore)),
			),
			newIdentifier,
			newClock,
			newTokenVerifier,
			newAuthenticator,
			newMessaging,
			newEventPublisher,
			application.NewOpenWalletService,
			application.NewPlaceBetService,
			newWagerApplier,
			application.NewInboxService,
			application.NewOutboxPublisher,
			application.NewReadService,
			httpapi.NewWalletHandler,
			httpapi.NewWagerTransactionHandler,
			newReadHandler,
			newHealthHandler,
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
	sqsEndpoint, err := requiredEnv("SQS_ENDPOINT")
	if err != nil {
		return config{}, err
	}
	return config{
		databaseURL:    databaseURL,
		httpAddr:       httpAddr,
		migrationsDir:  migrationsDir,
		oidcIssuer:     oidcIssuer,
		walletClientID: walletClientID,
		sqsEndpoint:    sqsEndpoint,
	}, nil
}

func newWagerApplier(bets *application.PlaceBetService) application.WagerApplier {
	return bets
}

func newEventPublisher(queues messaging) application.EventPublisher {
	return queues.events
}

func newMessaging(cfg config) (messaging, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	wagers, events, err := sqs.NewAPI(cfg.sqsEndpoint).Open(ctx)
	if err != nil {
		return messaging{}, err
	}
	return messaging{wagers: wagers, events: events}, nil
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

func newReadHandler(reads *application.ReadService, cfg config) *httpapi.ReadHandler {
	return httpapi.NewReadHandler(reads, cfg.walletClientID)
}

func newHealthHandler(pool *pgxpool.Pool, queues messaging) *httpapi.HealthHandler {
	return httpapi.NewHealthHandler(func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return errPostgresNotReady
		}
		wagerQueue, ok := queues.wagers.(*sqs.Queue)
		if !ok {
			return errSQSReadinessUnavailable
		}
		if err := wagerQueue.Ready(ctx); err != nil {
			return errSQSNotReady
		}
		return nil
	})
}

func newAuthenticator(tokens *oidcauth.Verifier, cfg config) *httpapi.Authenticator {
	return httpapi.NewAuthenticator(tokens, cfg.walletClientID)
}

func serve(
	lc fx.Lifecycle,
	authenticator *httpapi.Authenticator,
	handler http.Handler,
	bets *application.PlaceBetService,
	inbox *application.InboxService,
	publisher *application.OutboxPublisher,
	queues messaging,
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
			go runEvery(workerCtx, time.Second, "pending references", bets.AdvanceDueReferences)
			go runEvery(workerCtx, time.Second, "outbox", publisher.PublishDue)
			go consumeWagers(workerCtx, queues.wagers, inbox)
			go func() {
				if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
					slog.Error("http server", "error", err.Error())
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

func runEvery(ctx context.Context, interval time.Duration, operation string, work func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := work(ctx); err != nil && ctx.Err() == nil {
				slog.Error(operation, "error", err.Error())
			}
		}
	}
}

func consumeWagers(ctx context.Context, queue application.MessageQueue, handler application.WagerHandler) {
	for ctx.Err() == nil {
		err := application.Consume(ctx, queue, handler)
		if err == nil || ctx.Err() != nil {
			return
		}
		slog.Error("wager queue", "error", err.Error())
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
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
