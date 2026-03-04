package api

import (
	"net/http"
	"strconv"

	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

type StatsController struct{
	Mux *http.ServeMux
}

func (c *StatsController) LoadRoutes() {
	c.Mux.HandleFunc("/api/v1/stats/top-events", authMiddleware(handleTopEvents))
}

func handleTopEvents(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	stats, err := repository.GetTopEvents(limit)
	if err != nil {
		writeCBOR(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeCBOR(w, http.StatusOK, stats)
}
