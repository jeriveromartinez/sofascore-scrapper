package web

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/api/common"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/sofascore-scrapper/repository"
	"gorm.io/gorm"
)

type DomainController struct {
	Group *gin.RouterGroup
}

func (c *DomainController) LoadRoutes() {
	c.Group.GET("/domains", common.AuthMiddleware(), handleGetDomains)
	c.Group.GET("/domains/:id", common.AuthMiddleware(), handleGetDomain)
	c.Group.POST("/domains", common.AuthMiddleware(), handleCreateDomain)
	c.Group.PUT("/domains/:id", common.AuthMiddleware(), handleUpdateDomain)
	c.Group.DELETE("/domains/:id", common.AuthMiddleware(), handleDeleteDomain)
}

func handleGetDomains(c *gin.Context) {
	domains, err := repository.GetAllDomains()
	if err != nil {
		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	common.RespondProto(c, http.StatusOK, &pb.DomainList{Domains: common.DomainsToProto(domains)})
}

func handleGetDomain(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	domain, err := repository.GetDomainByID(id)
	if err != nil {
		common.RespondError(c, http.StatusNotFound, "domain not found")
		return
	}

	common.RespondProto(c, http.StatusOK, common.DomainToProto(*domain))
}

func handleCreateDomain(c *gin.Context) {
	var req pb.DomainRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Domain == "" || req.UserId == 0 {
		common.RespondError(c, http.StatusBadRequest, "domain and user_id are required")
		return
	}

	domain, err := repository.CreateDomain(req.Domain, uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.RespondError(c, http.StatusNotFound, "user not found")
			return
		}

		common.RespondError(c, http.StatusConflict, "could not create domain")
		return
	}

	common.RespondProto(c, http.StatusCreated, common.DomainToProto(*domain))
}

func handleUpdateDomain(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req pb.DomainRequest
	if err := common.ParseProtoBody(c, &req); err != nil || req.Domain == "" || req.UserId == 0 {
		common.RespondError(c, http.StatusBadRequest, "domain and user_id are required")
		return
	}

	domain, err := repository.UpdateDomain(id, req.Domain, uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.RespondError(c, http.StatusNotFound, "domain or user not found")
			return
		}

		common.RespondError(c, http.StatusConflict, "could not update domain")
		return
	}

	common.RespondProto(c, http.StatusOK, common.DomainToProto(*domain))
}

func handleDeleteDomain(c *gin.Context) {
	id, err := common.ParseID(c.Param("id"))
	if err != nil {
		common.RespondError(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := repository.DeleteDomain(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.RespondError(c, http.StatusNotFound, "domain not found")
			return
		}

		common.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	common.RespondProto(c, http.StatusOK, &pb.StatusMessage{Message: "domain deleted"})
}
