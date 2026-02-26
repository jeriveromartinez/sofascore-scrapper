package models

type EventsListResponse struct {
	Events []*APIEvent `json:"events"`
}