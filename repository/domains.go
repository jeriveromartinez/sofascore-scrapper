package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func GetAllDomains() ([]models.Domain, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	domains := make([]models.Domain, 0)
	result := db.Preload("User").Order("domain ASC").Find(&domains)
	return domains, result.Error
}

func GetDomainByID(id uint) (*models.Domain, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	var domain models.Domain
	result := db.Preload("User").First(&domain, id)
	return &domain, result.Error
}

func CreateDomain(domain string, userID uint) (*models.Domain, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	if err := db.First(&models.User{}, userID).Error; err != nil {
		return nil, err
	}

	record := &models.Domain{Domain: domain, UserID: userID}
	if err := db.Create(record).Error; err != nil {
		return nil, err
	}

	if err := db.Preload("User").First(record, record.ID).Error; err != nil {
		return nil, err
	}

	return record, nil
}

func UpdateDomain(id uint, domain string, userID uint) (*models.Domain, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	var record models.Domain
	if err := db.First(&record, id).Error; err != nil {
		return nil, err
	}

	if err := db.First(&models.User{}, userID).Error; err != nil {
		return nil, err
	}

	record.Domain = domain
	record.UserID = userID
	if err := db.Save(&record).Error; err != nil {
		return nil, err
	}

	if err := db.Preload("User").First(&record, record.ID).Error; err != nil {
		return nil, err
	}

	return &record, nil
}

func DeleteDomain(id uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	if err := db.First(&models.Domain{}, id).Error; err != nil {
		return err
	}

	return db.Delete(&models.Domain{}, id).Error
}
