// internal/scraper/catalog/model.go
package catalog

import "gorm.io/gorm"

type ScraperLeague struct {
	gorm.Model
	Source         string `gorm:"column:source;size:32;not null;default:'fotmob';index:idx_scraper_leagues_source"`
	SourceLeagueId string `gorm:"column:source_league_id;size:64;not null;uniqueIndex:idx_scraper_leagues_source_league"`
	Name           string `gorm:"column:name;type:longtext;not null"`
	Country        string `gorm:"column:country;size:8;not null;default:''"`
	Sport          string `gorm:"column:sport;size:32;not null;default:'football'"`
	Enabled        bool   `gorm:"column:enabled;not null;index:idx_scraper_leagues_enabled"`
}

func (ScraperLeague) TableName() string { return "scraper_leagues" }
