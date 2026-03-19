package common

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

func RespondProto(c *gin.Context, status int, v proto.Message) {
	data, err := proto.Marshal(v)
	if err != nil {
		c.String(http.StatusInternalServerError, "encoding error")
		return
	}
	c.Data(status, "application/x-protobuf", data)
}

func RespondError(c *gin.Context, status int, msg string) {
	c.JSON(status, map[string]string{"error": msg})
}

func ParseProtoBody(c *gin.Context, v proto.Message) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	return proto.Unmarshal(body, v)
}
