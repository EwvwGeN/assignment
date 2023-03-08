package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/gin-gonic/gin"
)

func (server *Server) checkJson(next gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var document models.Document
		data, _ := ioutil.ReadAll(ctx.Request.Body)
		if err := json.Unmarshal(data, &document); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
			return
		}
		var jsonData map[string]interface{}
		json.Unmarshal(data, &jsonData)
		ctx.Set("data", jsonData)
		next(ctx)
	}
}

func (server *Server) checkExist(next gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var document models.Document
		var jsonData map[string]interface{}
		id := ctx.Param("id")
		if id == "" {
			jsonData = ctx.GetStringMap("data")
			if jsonData["Id"] == nil {
				ctx.IndentedJSON(http.StatusNotFound, gin.H{"message": "document doesnt exist"})
				return
			}
			id = fmt.Sprintf("%.f", jsonData["Id"].(float64))
		}

		doc, found := server.findDoc(id)
		if !found {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"message": "document doesnt exist"})
			return
		}
		if jsonData == nil {
			document = doc.(models.Document)
			jsonByte, _ := json.Marshal(document)
			json.Unmarshal(jsonByte, &jsonData)
		}
		ctx.Set("data", jsonData)
		ctx.Set("id", id)
		next(ctx)
	}
}
