package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/EwvwGeN/assignment/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/restream/reindexer"
)

func (server *Server) createDoc() gin.HandlerFunc {
	return server.checkJson(func(ctx *gin.Context) {
		jsonData := ctx.GetStringMap("data")
		jsonStr, _ := json.Marshal(jsonData)
		var newDocument models.Document
		json.Unmarshal(jsonStr, &newDocument)
		childs := []int64{}
		// If child documents are passed to json, we translate them into an array of ids
		if jsonData["ChildList"] != nil {
			childs = util.ArrToInt64(jsonData["ChildList"].([]interface{}))
			newDocument.ChildList = nil
		}
		// Checking the possibility of using child documents
		if err := server.checkChild(0, childs); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("Can not create file: Can not add childs: %w", err).Error()})
			return
		}
		server.db.Insert(server.config.CollectionName, &newDocument, "id=serial()")
		// Writing to the json id of the created document
		jsonData["Id"] = newDocument.Id
		// Updating child documents of a document
		if err := server.updateChild(jsonData); err != nil {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		// Updating the list of child documents
		err := server.innerUpdateFields(newDocument.Id, map[string]interface{}{
			"ChildList": childs,
		})
		if err != nil {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		// Getting the document again to get all the changed fields and upload it to the cache
		doc, _ := server.findDoc(newDocument.Id)
		ctx.IndentedJSON(http.StatusOK, doc)
	})
}

func (server *Server) getAllDocs() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		query := server.db.Query(server.config.CollectionName)
		iterator := query.Exec()
		defer iterator.Close()
		for iterator.Next() {
			elem := iterator.Object().(*models.Document)
			ctx.IndentedJSON(http.StatusOK, elem)
		}
	}
}

// Get all documents that do not have a parent and bring them to the structure of the document with
// the expanded child elements
func (server *Server) getAllBigDocs(ctx *gin.Context) {
	query := server.db.Query(server.config.CollectionName).Where("parent_id", reindexer.EQ, 0)
	iterator := query.Exec()
	defer iterator.Close()
	for iterator.Next() {
		elem := iterator.Object().(*models.Document)
		bigDoc := server.bigDoc(elem)
		ctx.IndentedJSON(http.StatusOK, bigDoc)
	}
}

func (server *Server) getDocById() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		id := ctx.GetInt64("id")
		doc, _ := server.findDoc(id)
		ctx.IndentedJSON(http.StatusOK, doc)
	})
}

func (server *Server) updateDoc() gin.HandlerFunc {
	return server.checkJson(server.checkExist(func(ctx *gin.Context) {
		var jsonData map[string]interface{}
		id := ctx.GetInt64("id")
		jsonData = ctx.GetStringMap("data")
		jsonData["Id"] = id
		if err := server.updateChild(jsonData); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := server.updateDocFields(id, jsonData); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
	}))
}

func (server *Server) deleteDoc() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		var jsonData map[string]interface{}
		id := ctx.GetInt64("id")
		jsonData = ctx.GetStringMap("data")
		upperWg := new(sync.WaitGroup)
		upperWg.Add(2)
		// Start two goroutine to delete the lower documents and update the upper ones
		go func(upperWg *sync.WaitGroup) {
			defer upperWg.Done()
			server.innerDelete(id)
		}(upperWg)

		go func(upperWg *sync.WaitGroup) {
			defer upperWg.Done()
			parentId := int64(jsonData["ParentId"].(float64))
			if parentId != 0 {
				parentDoc, _ := server.findDoc(parentId)
				// Deleting the current document from child documents of the parent
				parentChild := func() []int64 {
					buffer := parentDoc.ChildList
					for i, value := range buffer {
						if value == id {
							return append(buffer[:i], buffer[i+1:]...)
						}
					}
					return nil
				}()
				server.innerUpdateFields(parentId, map[string]interface{}{
					"ChildList": parentChild,
				})
				server.updateDepth(parentDoc, parentChild)
			}
		}(upperWg)

		upperWg.Wait()
		ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
	})
}
