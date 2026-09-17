package tournaments

import "gorm.io/gorm"

type Tournament struct {
	gorm.Model
	// idx_tournaments_slug covers the cursor pagination in
	// repository.ListPage (ORDER BY slug ASC, id ASC) and the
	// dashboard load in repository.GetAll (ORDER BY slug ASC).
	// Without it every dashboard load walks the full tournaments
	// table and sorts in memory.
	Name   string `gorm:"column:name;type:longtext" json:"name"`
	Slug   string `gorm:"column:slug;type:longtext;index:idx_tournaments_slug" json:"slug"`
	Region string `gorm:"column:region;type:longtext" json:"region"`
}
