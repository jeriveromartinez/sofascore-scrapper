package api

import (
	"net/http"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

type EventController struct{
	Mux *http.ServeMux
}

func (c *EventController) LoadRoutes() {
	c.Mux.HandleFunc("/api/v1/events", authMiddleware(handleGetEvents))
}

func handleGetEvents(w http.ResponseWriter, r *http.Request) {
	db, err := database.GetDB()
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	date := r.URL.Query().Get("date")
	sport := r.URL.Query().Get("sport")

	query := db.Model(&models.SofaScoreEvent{})
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err == nil {
			start := t.Unix()
			end := t.Add(24 * time.Hour).Unix()
			query = query.Where("start_timestamp >= ? AND start_timestamp < ?", start, end)
		}
	}
	if sport != "" {
		query = query.Where("sport = ?", sport)
	}

	var events []models.SofaScoreEvent
	query.Find(&events)
	writeCBOR(w, http.StatusOK, events)
}
