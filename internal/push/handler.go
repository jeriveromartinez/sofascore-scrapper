package push

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
)

// TimerEntry is the minimal projection of a ScheduledPushTimer
// that the TimerStore index needs. Defined here so the handler can
// convert internal timers into the lighter Redis-friendly shape.
type TimerEntry struct {
	DeviceID uint
	FireAt   time.Time
}

// redisTimerStore is a forward declaration so the handler can
// call Enqueue/Remove without importing the redis package and
// creating a cycle. The concrete type lives in
// internal/platform/redis/timerstore.go and is wired by the app.
type redisTimerStore struct {
	enqFn func(scheduleID uint, entries []TimerEntry) error
	remFn func(scheduleID uint) error
}

func (r *redisTimerStore) enqueue(scheduleID uint, entries []TimerEntry) error {
	if r == nil || r.enqFn == nil {
		return nil
	}
	return r.enqFn(scheduleID, entries)
}

func (r *redisTimerStore) remove(scheduleID uint) error {
	if r == nil || r.remFn == nil {
		return nil
	}
	return r.remFn(scheduleID)
}

// Handler exposes the push-notifications REST surface. It is
// mounted on the webV1 group (auth required) by the app wire-up
// in internal/app/routes.go.
type Handler struct {
	svc *Service
	// callerFromContext extracts the user_id of the JWT holder.
	// Production passes auth.AuthMiddleware's Gin context key;
	// tests can plug a stub.
	callerFromContext func(c *gin.Context) (uint, bool)
	// timerStore is the Redis-backed index for scheduled push
	// timers. nil is acceptable: the schedule still fires
	// because the worker falls back to a DB scan, just slower.
	timerStore *redisTimerStore
	// logger is the slog handle for handler-level warnings. nil
	// falls back to slog.Default() so warnings are never lost.
	logger *slog.Logger
}

// HandlerDeps bundles the dependencies the handler needs. Kept
// in its own type so the wire-up site is greppable.
type HandlerDeps struct {
	// CallerID extracts the authenticated user id from the
	// request context. Required.
	CallerID func(c *gin.Context) (uint, bool)
}

// NewHandler returns a handler backed by the given service.
func NewHandler(svc *Service, deps HandlerDeps) *Handler {
	if deps.CallerID == nil {
		panic("push.NewHandler: deps.CallerID is required")
	}
	return &Handler{svc: svc, callerFromContext: deps.CallerID}
}

// SetTimerStore wires the Redis-backed timer index into the
// handler. Called by the app wire-up; nil-safe (handler just
// skips the enqueue and the worker falls back to DB scan).
func (h *Handler) SetTimerStore(store *redisTimerStore, logger *slog.Logger) {
	h.timerStore = store
	if logger != nil {
		h.logger = logger
	}
}

// timersToFireEntries converts the freshly inserted
// ScheduledPushTimer rows into the lighter TimerEntry projection.
func timersToFireEntries(rows []ScheduledPushTimer) []TimerEntry {
	out := make([]TimerEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, TimerEntry{DeviceID: r.DeviceID, FireAt: r.FireAt})
	}
	return out
}

// RegisterRoutes wires the push REST surface under the given
// group. All endpoints require the caller's JWT (the
// AuthMiddleware on the group enforces that).
func (h *Handler) RegisterRoutes(group *gin.RouterGroup, authMW gin.HandlerFunc) {
	p := group.Group("/pushes", authMW)
	p.POST("", h.handleCreateImmediate)
	p.GET("", h.handleList)
	p.GET("/metrics/aggregate", h.handleMetricsAggregate)
	p.GET("/metrics/campaign/:id", h.handleCampaignMetrics)
	p.GET("/:id", h.handleGet)
	p.POST("/schedules", h.handleCreateSchedule)
	p.GET("/schedules", h.handleListSchedules)
	p.GET("/schedules/:id", h.handleGetSchedule)
	p.PATCH("/schedules/:id", h.handleUpdateSchedule)
	p.DELETE("/schedules/:id", h.handleDeleteSchedule)
}

func (h *Handler) handleCreateImmediate(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req pb.CreateImmediatePushRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	msg, _, err := h.svc.CreateImmediate(c.Request.Context(), callerID, callerID, uint32SliceToUint(req.DomainIds), req.Payload)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	server.RespondProto(c, http.StatusCreated, &pb.PushMessage{
		Id:         uint32(msg.ID),
		CreatedAt:  msg.CreatedAt.Format(time.RFC3339),
		UserId:     uint32(msg.UserID),
		Category:   CategoryToProto(msg.Category),
		Title:      msg.Title,
		Body:       msg.Body,
		ImageUrl:   msg.ImageURL,
		DeepLink:   msg.DeepLink,
		Priority:   PriorityToProto(msg.Priority),
		TtlSeconds: int32(msg.TTLSeconds),
		Data:       map[string]string(msg.DataJSON),
		Source:     string(msg.Source),
		DomainIds:  req.DomainIds,
	})
}

func (h *Handler) handleGet(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	// For now, Get is just a thin wrapper around the repo. The
	// full detail+metrics response is wired in a follow-up.
	msg, err := h.svc.repo.GetPushMessageByID(c.Request.Context(), id, callerID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	targets, err := h.svc.repo.GetPushMessageTargets(c.Request.Context(), msg.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	domainIDs := make([]uint32, 0, len(targets))
	for _, t := range targets {
		domainIDs = append(domainIDs, uint32(t.DomainID))
	}
	server.RespondProto(c, http.StatusOK, PushMessageToProto(*msg, domainIDs))
}

func (h *Handler) handleList(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	source := c.Query("source") // optional: "immediate" | "scheduled"
	rows, hasMore, err := h.svc.repo.ListPushMessagesByUser(c.Request.Context(), callerID, source, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp := &pb.PushMessagePage{
		Data: make([]*pb.PushMessage, 0, len(rows)),
	}
	if len(rows) > 0 {
		ids := make([]uint, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		targetsByID, terr := h.svc.repo.GetPushMessageTargetsByMessageIDs(c.Request.Context(), ids)
		if terr != nil {
			server.RespondError(c, http.StatusInternalServerError, terr.Error())
			return
		}
		for i := range rows {
			targets := targetsByID[rows[i].ID]
			domainIDs := make([]uint32, 0, len(targets))
			for _, t := range targets {
				domainIDs = append(domainIDs, uint32(t.DomainID))
			}
			resp.Data = append(resp.Data, PushMessageToProto(rows[i], domainIDs))
		}
	}
	if hasMore && len(rows) > 0 {
		// Cursor is the id of the last row. The frontend passes it
		// back as ?after_id=. The repo's ListPage handles it.
		last := rows[len(rows)-1]
		resp.Page = &pb.CursorPageInfo{
			NextCursor: strconv.FormatUint(uint64(last.ID), 10),
			HasMore:    true,
		}
	}
	server.RespondProto(c, http.StatusOK, resp)
}

func (h *Handler) handleCreateSchedule(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req pb.CreateScheduleRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	var runAt *time.Time
	if req.RunAt != "" {
		t, err := time.Parse(time.RFC3339, req.RunAt)
		if err != nil {
			server.RespondError(c, http.StatusBadRequest, "run_at must be RFC3339")
			return
		}
		runAt = &t
	}

	// The dashboard signals the timezone behavior via query
	// params (?tz_mode=...&timezone=...). The proto wire
	// format is unchanged so the existing API contract stays
	// stable. Defaults preserve the previous behavior (cron in
	// UTC) so older dashboard builds continue to work.
	tzMode := TimezoneModeShared
	switch c.Query("tz_mode") {
	case "device_local":
		tzMode = TimezoneModeDeviceLocal
	}
	timezone := c.Query("timezone")
	if timezone == "" {
		timezone = "UTC"
	}

	sched, timers, err := h.svc.CreateSchedule(c.Request.Context(), callerID, callerID, uint32SliceToUint(req.DomainIds), req.Payload, req.ScheduleType, runAt, req.CronExpr, tzMode, timezone)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if h.timerStore != nil {
		if err := h.timerStore.enqueue(sched.ID, timersToFireEntries(timers)); err != nil {
			if h.logger != nil {
				h.logger.Warn("push: enqueue schedule timers failed",
					slog.Uint64("schedule_id", uint64(sched.ID)),
					slog.String("error", err.Error()))
			}
		}
	}
	server.RespondProto(c, http.StatusCreated, ScheduledPushToProto(*sched, req.DomainIds))
}

func (h *Handler) handleListSchedules(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	rows, hasMore, err := h.svc.repo.ListScheduledPushesByUser(c.Request.Context(), callerID, limit)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp := &pb.ScheduledPushPage{Data: make([]*pb.ScheduledPush, 0, len(rows))}
	if len(rows) > 0 {
		ids := make([]uint, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		targetsByID, terr := h.svc.repo.GetScheduledPushTargetsByScheduledIDs(c.Request.Context(), ids)
		if terr != nil {
			server.RespondError(c, http.StatusInternalServerError, terr.Error())
			return
		}
		for i := range rows {
			targets := targetsByID[rows[i].ID]
			domainIDs := make([]uint32, 0, len(targets))
			for _, t := range targets {
				domainIDs = append(domainIDs, uint32(t.DomainID))
			}
			resp.Data = append(resp.Data, ScheduledPushToProto(rows[i], domainIDs))
		}
	}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		resp.Page = &pb.CursorPageInfo{
			NextCursor: strconv.FormatUint(uint64(last.ID), 10),
			HasMore:    true,
		}
	}
	server.RespondProto(c, http.StatusOK, resp)
}

func (h *Handler) handleGetSchedule(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	sched, err := h.svc.repo.GetScheduledPushByID(c.Request.Context(), id, callerID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	targets, terr := h.svc.repo.GetScheduledPushTargets(c.Request.Context(), sched.ID)
	if terr != nil {
		writeServiceError(c, terr)
		return
	}
	domainIDs := make([]uint32, 0, len(targets))
	for _, t := range targets {
		domainIDs = append(domainIDs, uint32(t.DomainID))
	}
	server.RespondProto(c, http.StatusOK, ScheduledPushToProto(*sched, domainIDs))
}

func (h *Handler) handleUpdateSchedule(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req pb.UpdateScheduleRequest
	if err := server.ParseProtoBody(c, &req); err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	sched, err := h.svc.repo.UpdateScheduledPushActive(c.Request.Context(), id, callerID, req.IsActive)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	targets, terr := h.svc.repo.GetScheduledPushTargets(c.Request.Context(), sched.ID)
	if terr != nil {
		writeServiceError(c, terr)
		return
	}
	domainIDs := make([]uint32, 0, len(targets))
	for _, t := range targets {
		domainIDs = append(domainIDs, uint32(t.DomainID))
	}
	server.RespondProto(c, http.StatusOK, ScheduledPushToProto(*sched, domainIDs))
}

func (h *Handler) handleDeleteSchedule(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.repo.DeleteScheduledPush(c.Request.Context(), id, callerID); err != nil {
		writeServiceError(c, err)
		return
	}
	server.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "schedule deleted"})
}

func (h *Handler) handleMetricsAggregate(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	snap, err := h.svc.repo.AggregateMetricsForUser(c.Request.Context(), callerID)
	if err != nil {
		server.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondProto(c, http.StatusOK, snap)
}

func (h *Handler) handleCampaignMetrics(c *gin.Context) {
	callerID, ok := h.callerFromContext(c)
	if !ok {
		server.RespondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := server.ParseID(c.Param("id"))
	if err != nil {
		server.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}
	snap, err := h.svc.repo.CampaignMetrics(c.Request.Context(), id, callerID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	server.RespondProto(c, http.StatusOK, snap)
}

// writeServiceError maps the service's sentinel errors to HTTP
// status codes. Centralized so every handler uses the same
// translation table.
func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		server.RespondError(c, http.StatusForbidden, "forbidden")
	case errors.Is(err, ErrNotFound):
		server.RespondError(c, http.StatusNotFound, "not found")
	case errors.Is(err, ErrInvalidPayload):
		server.RespondError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidSchedule):
		server.RespondError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrDisabledFeature):
		server.RespondError(c, http.StatusForbidden, "notifications disabled for this user")
	default:
		server.RespondError(c, http.StatusInternalServerError, err.Error())
	}
}

// uint32SliceToUint narrows the proto's []uint32 back to the
// service's []uint. The proto field is uint32 because the wire
// format only supports 32-bit ids; the internal representation
// stays at platform width so we can swap drivers without churning
// the code that consumes the slice.
func uint32SliceToUint(ids []uint32) []uint {
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		out = append(out, uint(id))
	}
	return out
}
