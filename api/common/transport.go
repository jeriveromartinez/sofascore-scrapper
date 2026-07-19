package common

import (
	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/server"
	"google.golang.org/protobuf/proto"
)

func RespondProto(c *gin.Context, status int, v proto.Message) {
	server.RespondProto(c, status, v)
}

func RespondError(c *gin.Context, status int, msg string) {
	server.RespondError(c, status, msg)
}

func ParseProtoBody(c *gin.Context, v proto.Message) error {
	return server.ParseProtoBody(c, v)
}
