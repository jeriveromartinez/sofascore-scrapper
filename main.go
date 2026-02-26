package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/httpcli"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
)

func main() {
	models.Migrate()
	var list models.EventsListResponse
	body := httpcli.LoadData(httpcli.FOOTBALL, time.Now().Add(time.Hour*24))
	if json.Unmarshal(body, &list) != nil {
		panic("Error parsing JSON")
	}

	repository.SaveSofaScoreEvent(list.Events)
	fmt.Println("Import ready")
}
