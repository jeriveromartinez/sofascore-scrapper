package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	internalDomains "github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
)

func domainsRepo() (*internalDomains.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return internalDomains.NewRepository(db), nil
}

func GetAllDomains() ([]internalDomains.Domain, error) {
	repo, err := domainsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAll()
}

func GetDomainByID(id uint) (*internalDomains.Domain, error) {
	repo, err := domainsRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByID(id)
}

func CreateDomain(domain string, userID uint) (*internalDomains.Domain, error) {
	repo, err := domainsRepo()
	if err != nil {
		return nil, err
	}
	return repo.Create(domain, userID)
}

func UpdateDomain(id uint, domain string, userID uint) (*internalDomains.Domain, error) {
	repo, err := domainsRepo()
	if err != nil {
		return nil, err
	}
	return repo.Update(id, domain, userID)
}

func DeleteDomain(id uint) error {
	repo, err := domainsRepo()
	if err != nil {
		return err
	}
	return repo.Delete(id)
}
