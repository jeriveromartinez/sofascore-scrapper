package app

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/scraper"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func NewRouter(db *gorm.DB, redisClient *goredis.Client, cfg config.Config, tokens *auth.TokenService) *gin.Engine {
	if cfg.ScrapeBatchSize <= 0 {
		cfg.ScrapeBatchSize = 500
	}
	router := gin.New()

	router.Use(gin.Recovery())
	router.Use(requestID())
	router.Use(server.BodyLimit())
	router.Use(server.CORS())
	router.Use(gin.Logger())

	rl := server.RateLimit(redisClient)

	appV1 := router.Group("/api/app/v1", rl)
	webV1 := router.Group("/api/web/v1")

	appMw := devices.AppMiddleware(db)
	authMw := auth.AuthMiddleware(tokens)
	authThenRl := func(c *gin.Context) {
		authMw(c)
		if !c.IsAborted() {
			rl(c)
		}
	}

	apkRepo := apk.NewRepository(db)
	apkDownloadCounter := apk.NewDownloadCounter(redisClient, db)
	apkAppHandler := apk.NewAppHandler(apkRepo, apkDownloadCounter)
	apkAppHandler.RegisterRoutes(appV1)

	apkAdminHandler := apk.NewAdminHandler(apkRepo)
	apkAdminHandler.RegisterRoutes(webV1, apk.AdminHandlerDeps{AuthMiddleware: authThenRl})

	apkUploadStateStore := apk.NewUploadStateStore(redisClient)
	apkChunkStore := apk.NewChunkStore(cfg.APKStoragePath)
	apkUploadService := apk.NewUploadService(apkUploadStateStore, apkChunkStore, apkRepo, db)
	apkUploadHandler := apk.NewUploadHandler(apkUploadService, apkUploadStateStore)
	apkUploadHandler.RegisterRoutes(webV1, apk.AdminHandlerDeps{AuthMiddleware: authThenRl})

	devRepo := devices.NewRepository(db)
	playbackRepo := playback.NewRepository(db)

	devicesAppHandler := devices.NewAppHandler(devRepo)
	devicesAppHandler.RegisterRoutes(appV1)

	playbackService := playback.NewService(playbackRepo)
	playbackAppHandler := playback.NewAppHandler(playbackService)
	playbackAppHandler.RegisterRoutes(appV1, playback.PlaybackAppHandlerDeps{AppMiddleware: appMw})

	devicesAdminHandler := devices.NewAdminHandler(devRepo)
	devicesAdminHandler.RegisterRoutes(webV1, devices.AdminHandlerDeps{AuthMiddleware: authThenRl})

	playbackAdminHandler := playback.NewAdminHandler(playbackRepo)
	playbackAdminHandler.RegisterRoutes(webV1, playback.AdminHandlerDeps{AuthMiddleware: authThenRl})

	reportingRepo := reporting.NewRepository(db)
	crashHandler := reporting.NewCrashHandler(reportingRepo)
	crashHandler.RegisterRoutes(appV1)

	statsHandler := reporting.NewStatsHandler(reportingRepo)
	statsHandler.RegisterRoutes(webV1, reporting.StatsHandlerDeps{AuthMiddleware: authThenRl})

	eventsRepo := events.NewRepository(db)
	eventsCache := events.NewCurrentEventsCache(redisClient)
	eventsEpoch := events.NewEpochStore(redisClient)
	eventsService := events.NewService(eventsRepo, eventsCache, eventsEpoch)
	eventsAppHandler := events.NewAppHandler(eventsService)
	eventsAppHandler.RegisterRoutes(appV1, events.AppHandlerDeps{AppMiddleware: appMw})

	eventsAdminHandler := events.NewAdminHandler(db)
	eventsAdminHandler.RegisterRoutes(webV1, events.AdminHandlerDeps{AuthMiddleware: authThenRl})

	logoHandler := events.NewLogoHandler()
	logoHandler.RegisterRoutes(appV1)

	userRepo := users.NewRepository(db)
	userHandler := users.NewHandler(userRepo)
	userHandler.RegisterUserRoutes(webV1, users.HandlerDeps{AuthMiddleware: authThenRl})

	authRepo := auth.NewAuthRepository(db)
	invitationStore := auth.NewInvitationStore(redisClient)
	authHandler := auth.NewAuthHandler(authRepo, userRepo, tokens, invitationStore)
	authHandler.RegisterAuthRoutes(webV1, rl)

	tournamentRepo := tournaments.NewRepository(db)
	tournamentHandler := tournaments.NewHandler(tournamentRepo)
	tournamentHandler.RegisterRoutes(webV1, tournaments.HandlerDeps{AuthMiddleware: authThenRl})

	deviceAssignmentsRepo := tournaments.NewDeviceAssignmentsRepository(db)
	deviceAssignmentsHandler := tournaments.NewDeviceAssignmentsHandler(deviceAssignmentsRepo)
	deviceAssignmentsHandler.SetOnChange(func(ctx context.Context) error {
		_, err := eventsEpoch.Increment(ctx)
		return err
	})
	deviceAssignmentsHandler.RegisterRoutes(webV1, tournaments.DeviceAssignmentsHandlerDeps{AuthMiddleware: authThenRl})

	globalConfigRepo := tournaments.NewGlobalConfigRepository(db)
	globalConfigHandler := tournaments.NewGlobalConfigHandler(globalConfigRepo)
	globalConfigHandler.SetOnChange(func(ctx context.Context) error {
		_, err := eventsEpoch.Increment(ctx)
		return err
	})
	globalConfigHandler.RegisterRoutes(webV1, tournaments.GlobalConfigHandlerDeps{AuthMiddleware: authThenRl})

	domainRepo := domains.NewRepository(db)
	domainHandler := domains.NewHandler(domainRepo)
	domainHandler.RegisterRoutes(webV1, domains.HandlerDeps{AuthMiddleware: authThenRl})

	server.RegisterDashboardRoutes(router)

	return router
}

func buildSchedulerDeps(db *gorm.DB, batchSize int, concurrency int, epoch *events.EpochStore) (*scraper.Service, *reporting.AggregationRepository) {
	client, err := scraper.NewClient(scraper.ClientConfig{})
	if err != nil {
		panic("scraper: failed to create client: " + err.Error())
	}
	eventsRepo := events.NewRepository(db)
	scrapeSvc, err := scraper.NewService(eventsRepo, client, batchSize, concurrency)
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
