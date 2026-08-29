package app

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/push"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/realtime"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, redisClient *goredis.Client, cfg config.Config, tokens *auth.TokenService, logoScheduler events.TeamLogoScheduler, hub *realtime.Hub, pushSvc *push.Service) *gin.Engine {
	if cfg.ScrapeBatchSize <= 0 {
		cfg.ScrapeBatchSize = 500
	}
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.HTTP.TrustedProxies); err != nil {
		panic("app: invalid trusted proxies: " + err.Error())
	}

	router.Use(gin.Recovery())
	router.Use(server.RequestID())
	router.Use(server.BodyLimit())
	router.Use(server.CORS())
	router.Use(server.PrometheusMiddleware())
	router.Use(server.SlogLogger())

	healthChecker := server.NewHealthChecker()
	healthChecker.Register("database", func(ctx context.Context) error { return db.WithContext(ctx).Exec("SELECT 1").Error })
	healthChecker.Register("redis", func(ctx context.Context) error { return redisClient.Ping(ctx).Err() })

	router.GET("/health/live", healthChecker.LivenessHandler())
	router.GET("/health/ready", healthChecker.ReadinessHandler())
	router.GET("/metrics", server.MetricsHandler())

	rl := server.RateLimit(redisClient)

	appV1 := router.Group("/api/app/v1", rl)
	webV1 := router.Group("/api/web/v1")

	appMw := devices.AppMiddleware(db)
	authMw := auth.AuthMiddleware(tokens)
	userRepo := users.NewRepository(db)
	adminMw := auth.RequireAdmin(userRepo)
	// Order matters: authenticate, then rate-limit (fail-closed before any
	// expensive work), then the admin check which hits the database.
	adminThenRl := func(c *gin.Context) {
		authMw(c)
		if c.IsAborted() {
			return
		}
		rl(c)
		if c.IsAborted() {
			return
		}
		adminMw(c)
	}

	apkRepo := apk.NewRepository(db)
	apkDownloadCounter := apk.NewDownloadCounter(redisClient, db)
	apkAppHandler := apk.NewAppHandler(apkRepo, apkDownloadCounter)
	apkAppHandler.RegisterRoutes(appV1)

	apkAdminHandler := apk.NewAdminHandler(apkRepo)
	apkAdminHandler.RegisterRoutes(webV1, apk.AdminHandlerDeps{AuthMiddleware: adminThenRl})

	apkUploadStateStore := apk.NewUploadStateStore(redisClient)
	apkChunkStore := apk.NewChunkStore(cfg.APKStoragePath)
	apkUploadService := apk.NewUploadService(apkUploadStateStore, apkChunkStore, apkRepo, db)
	apkUploadHandler := apk.NewUploadHandler(apkUploadService, apkUploadStateStore)
	apkUploadHandler.RegisterRoutes(webV1, apk.AdminHandlerDeps{AuthMiddleware: adminThenRl})

	devRepo := devices.NewRepository(db)
	playbackRepo := playback.NewRepository(db)

	devicesAppHandler := devices.NewAppHandler(devRepo)
	devicesAppHandler.RegisterRoutes(appV1)

	playbackService := playback.NewService(playbackRepo)
	playbackAppHandler := playback.NewAppHandler(playbackService)
	playbackAppHandler.RegisterRoutes(appV1, playback.PlaybackAppHandlerDeps{AppMiddleware: appMw})

	devicesAdminHandler := devices.NewAdminHandler(devRepo)
	devicesAdminHandler.RegisterRoutes(webV1, devices.AdminHandlerDeps{AuthMiddleware: adminThenRl})

	playbackAdminHandler := playback.NewAdminHandler(playbackRepo)
	playbackAdminHandler.RegisterRoutes(webV1, playback.AdminHandlerDeps{AuthMiddleware: adminThenRl})

	reportingRepo := reporting.NewRepository(db)
	crashHandler := reporting.NewCrashHandler(reportingRepo)
	crashHandler.RegisterRoutes(appV1)

	statsHandler := reporting.NewStatsHandler(reportingRepo)
	statsHandler.RegisterRoutes(webV1, reporting.StatsHandlerDeps{AuthMiddleware: adminThenRl})

	eventsRepo := events.NewRepositoryWithLogoScheduler(db, logoScheduler)
	eventsCache := events.NewCurrentEventsCache(redisClient)
	eventsEpoch := events.NewEpochStore(redisClient)
	eventsService := events.NewService(eventsRepo, eventsCache, eventsEpoch)
	eventsAppHandler := events.NewAppHandler(eventsService)
	eventsAppHandler.RegisterRoutes(appV1, events.AppHandlerDeps{AppMiddleware: appMw})

	eventsAdminHandler := events.NewAdminHandler(eventsRepo)
	eventsAdminHandler.RegisterRoutes(webV1, events.AdminHandlerDeps{AuthMiddleware: adminThenRl})

	logoHandler := events.NewLogoHandler()
	logoHandler.RegisterRoutes(appV1)

	userHandler := users.NewHandler(userRepo)
	userHandler.RegisterUserRoutes(webV1, users.HandlerDeps{AuthMiddleware: adminThenRl})

	authRepo := auth.NewAuthRepository(db)
	invitationStore := auth.NewInvitationStore(redisClient)
	authHandler := auth.NewAuthHandler(authRepo, userRepo, tokens, invitationStore)
	authHandler.RegisterAuthRoutes(webV1, rl, adminMw)

	tournamentRepo := tournaments.NewRepository(db)
	tournamentHandler := tournaments.NewHandler(tournamentRepo)
	tournamentHandler.RegisterRoutes(webV1, tournaments.HandlerDeps{AuthMiddleware: adminThenRl})

	deviceAssignmentsRepo := tournaments.NewDeviceAssignmentsRepository(db)
	deviceAssignmentsHandler := tournaments.NewDeviceAssignmentsHandler(deviceAssignmentsRepo)
	deviceAssignmentsHandler.SetOnChange(func(ctx context.Context) error {
		_, err := eventsEpoch.Increment(ctx)
		return err
	})
	deviceAssignmentsHandler.RegisterRoutes(webV1, tournaments.DeviceAssignmentsHandlerDeps{AuthMiddleware: adminThenRl})

	globalConfigRepo := tournaments.NewGlobalConfigRepository(db)
	globalConfigHandler := tournaments.NewGlobalConfigHandler(globalConfigRepo)
	globalConfigHandler.SetOnChange(func(ctx context.Context) error {
		_, err := eventsEpoch.Increment(ctx)
		return err
	})
	globalConfigHandler.RegisterRoutes(webV1, tournaments.GlobalConfigHandlerDeps{AuthMiddleware: adminThenRl})

	domainRepo := domains.NewRepository(db)
	domainHandler := domains.NewHandler(domainRepo)
	domainHandler.RegisterRoutes(webV1, domains.HandlerDeps{AuthMiddleware: adminThenRl})

	// Push notifications REST surface. The service was created
	// in app.New so it can be shared with the realtime WS handler
	// (which uses it as the AckHandler for incoming WsPushAck
	// frames).
	if pushSvc != nil {
		pushHandler := push.NewHandler(pushSvc, push.HandlerDeps{
			CallerID: callerIDFromContext,
		})
		pushHandler.RegisterRoutes(webV1, adminThenRl)
	}

	// Realtime (WebSocket) endpoint for the Flutter client. The
	// handler authenticates the device via APP-XIPTV (or ?token=)
	// and upgrades the request; no JWT, no appMw — the Flutter app
	// does not authenticate as a user.
	wsAuthenticator := realtime.NewAuthenticator(db)
	ackHandler := realtime.AckHandler(func(string) {})
	if pushSvc != nil {
		// Wrap the service's OnAck to the realtime AckHandler
		// signature (no ctx, no return). The connection's
		// background context is what the service uses anyway, so
		// we just call it with context.Background().
		pushSvc := pushSvc
		ackHandler = func(messageID string) {
			_ = pushSvc.OnAck(context.Background(), messageID)
		}
	}
	wsHandler := realtime.Handler(realtime.HandlerConfig{
		Authenticator: wsAuthenticator,
		Hub:           hub,
		Logger:        slog.Default(),
		AckHandler:    ackHandler,
	})
	appV1.GET("/ws", wsHandler)

	server.RegisterDashboardRoutes(router)

	return router
}

func buildSchedulerDeps(db *gorm.DB, batchSize int, concurrency int, epoch *events.EpochStore, logoScheduler events.TeamLogoScheduler, logger *slog.Logger) (*scraper.Service, *reporting.AggregationRepository) {
	client, err := scraper.NewClient(scraper.ClientConfig{})
	if err != nil {
		panic("scraper: failed to create client: " + err.Error())
	}
	eventsRepo := events.NewRepositoryWithLogoScheduler(db, logoScheduler)
	scrapeSvc, err := scraper.NewService(eventsRepo, client, batchSize, concurrency, logger)
	if err != nil {
		panic("scraper: failed to create service: " + err.Error())
	}
	scrapeSvc.SetOnScrapeComplete(func(ctx context.Context) error {
		_, err := epoch.Increment(ctx)
		return err
	})
	aggRepo := reporting.NewAggregationRepository(db)
	return scrapeSvc, aggRepo
}

// callerIDFromContext extracts the authenticated user id from the
// gin context. auth.AuthMiddleware stashes the id under the
// "userID" key; this closure is passed to push.NewHandler as the
// CallerID dependency. The closure lives in app/ so that the
// push package does not have to import auth (which itself imports
// users, creating a cycle if the handler were in users).
func callerIDFromContext(c *gin.Context) (uint, bool) {
	v, ok := c.Get("userID")
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	if !ok {
		// The auth package sometimes stores other integer types
		// depending on the token variant; accept int64 and uint32
		// as a defensive measure.
		switch n := v.(type) {
		case int64:
			return uint(n), true
		case uint32:
			return uint(n), true
		case int:
			return uint(n), true
		}
		return 0, false
	}
	return id, true
}
