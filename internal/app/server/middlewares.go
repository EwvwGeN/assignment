package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/gin-gonic/gin"
)

// Attempt to write json to the structure and, if successful, run the following function
func (server *Server) checkJson(next gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var document models.Document
		data, _ := ioutil.ReadAll(ctx.Request.Body)
		if err := json.Unmarshal(data, &document); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": InvalidRequest.Error()})
			return
		}
		var jsonData map[string]interface{}
		json.Unmarshal(data, &jsonData)
		ctx.Set("data", jsonData)
		next(ctx)
	}
}

// Attempt to get a document from id or from json and, if successful, launch the following function
func (server *Server) checkExist(next gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var document *models.Document
		var jsonData map[string]interface{}
		id := ctx.Param("id")
		if id == "" {
			jsonData = ctx.GetStringMap("data")
			if jsonData["Id"] == nil {
				ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": NullId.Error()})
				return
			}
			id = fmt.Sprintf("%.f", jsonData["Id"].(float64))
		}
		buffer, _ := strconv.Atoi(id)
		docId := int64(buffer)
		doc, found := server.findDoc(docId)
		if !found {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": DocumentNotExist.Error()})
			return
		}
		if jsonData == nil {
			document = doc
			jsonByte, _ := json.Marshal(document)
			json.Unmarshal(jsonByte, &jsonData)
		}
		ctx.Set("data", jsonData)
		ctx.Set("id", docId)
		next(ctx)
	}
}
