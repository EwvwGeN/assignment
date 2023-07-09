package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/EwvwGeN/assignment/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/restream/reindexer/v3"
	"go.opentelemetry.io/otel"
)

func (server *Server) createDoc() gin.HandlerFunc {
	return server.checkJson(func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("CreateDoc").Start(ctx.Request.Context(), "Create doc handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
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

		tx, err := server.db.BeginTx(server.config.CollectionName)
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		actionSaver := server.cache.NewActionSaver()

		// Updating child documents of a document
		if err := server.updateChild(tx, actionSaver.Channel, jsonData); err != nil {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		// Updating the list of child documents
		server.innerUpdateFields(tx, actionSaver.Channel, newDocument.Id, map[string]interface{}{
			"ChildList": childs,
		})
		if err != nil {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		if err := tx.Commit(); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		actionSaver.Commit()
		// Getting the document again to get all the changed fields and upload it to the cache
		doc, _ := server.findDoc(newDocument.Id)
		ctx.IndentedJSON(http.StatusCreated, doc)
	})
}

func (server *Server) getAllDocs() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("GetAllDocs").Start(ctx.Request.Context(), "Get all docs handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
		page, err := strconv.Atoi(ctx.DefaultQuery("page", "0"))
		if err != nil || page < 0 {
			ctx.AbortWithStatus(http.StatusBadRequest)
			return
		}
		limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "10"))
		if err != nil || limit < 0 {
			ctx.AbortWithStatus(http.StatusBadRequest)
			return
		}
		query := server.db.Query(server.config.CollectionName)
		if page != 0 {
			query = query.Limit(limit).Offset((page - 1) * limit)
		}
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
func (server *Server) getAllBigDocs() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("GetAllBigDocs").Start(ctx.Request.Context(), "Get all big docs handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
		page, err := strconv.Atoi(ctx.DefaultQuery("page", "0"))
		if err != nil || page < 0 {
			ctx.AbortWithStatus(http.StatusBadRequest)
			return
		}
		limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "10"))
		if err != nil || limit < 0 {
			ctx.AbortWithStatus(http.StatusBadRequest)
			return
		}
		query := server.db.Query(server.config.CollectionName).Where("ParentId", reindexer.EQ, 0)
		if page != 0 {
			query = query.Limit(limit).Offset((page - 1) * limit)
		}

		iterator := query.Exec()
		defer iterator.Close()
		for iterator.Next() {
			elem := iterator.Object().(*models.Document)
			bigDoc := server.bigDoc(elem)
			if bigDoc.ChildList != nil {
				sort.Slice(bigDoc.ChildList, func(i, j int) bool {
					return bigDoc.ChildList[i].Sort > bigDoc.ChildList[j].Sort
				})
			}
			ctx.IndentedJSON(http.StatusOK, bigDoc)
		}
	}
}

func (server *Server) getBigDocById() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("GetBigDocById").Start(ctx.Request.Context(), "Get big doc handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
		id := ctx.GetInt64("id")
		doc, _ := server.findDoc(id)
		for doc.ParentId != 0 {
			doc, _ = server.findDoc(doc.ParentId)
		}
		bigDoc := server.bigDoc(doc)
		if bigDoc.ChildList != nil {
			sort.Slice(bigDoc.ChildList, func(i, j int) bool {
				return bigDoc.ChildList[i].Sort > bigDoc.ChildList[j].Sort
			})
		}
		ctx.IndentedJSON(http.StatusOK, bigDoc)
	})
}

func (server *Server) getDocById() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("GetDocById").Start(ctx.Request.Context(), "Get doc handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
		id := ctx.GetInt64("id")
		doc, _ := server.findDoc(id)
		ctx.IndentedJSON(http.StatusOK, doc)
	})
}

func (server *Server) updateDoc() gin.HandlerFunc {
	return server.checkJson(server.checkExist(func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("UpdateDoc").Start(ctx.Request.Context(), "Update doc handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
		var jsonData map[string]interface{}
		id := ctx.GetInt64("id")
		jsonData = ctx.GetStringMap("data")
		jsonData["Id"] = id

		actionSaver := server.cache.NewActionSaver()
		tx, _ := server.db.BeginTx(server.config.CollectionName)
		if err := server.updateChild(tx, actionSaver.Channel, jsonData); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := server.updateDocFields(tx, actionSaver.Channel, id, jsonData); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := tx.Commit(); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		actionSaver.Commit()

		ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
	}))
}

func (server *Server) deleteDoc() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		traceCtx, span_one := otel.Tracer("DeleteDoc").Start(ctx.Request.Context(), "Delete doc handler")
		defer span_one.End()
		*ctx.Request = *ctx.Request.WithContext(traceCtx)
		server.db.WithContext(ctx.Request.Context())
		var jsonData map[string]interface{}
		id := ctx.GetInt64("id")
		jsonData = ctx.GetStringMap("data")
		actionSaver := server.cache.NewActionSaver()
		tx, _ := server.db.BeginTx(server.config.CollectionName)
		upperWg := new(sync.WaitGroup)
		upperWg.Add(2)
		// Start two goroutine to delete the lower documents and update the upper ones
		go func(upperWg *sync.WaitGroup) {
			defer upperWg.Done()
			server.innerDelete(tx, actionSaver.Channel, id)
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
				server.innerUpdateFields(tx, actionSaver.Channel, parentId, map[string]interface{}{
					"ChildList": parentChild,
				})
				server.updateDepth(tx, actionSaver.Channel, parentDoc, parentChild)
			}
		}(upperWg)
		upperWg.Wait()

		if err := tx.Commit(); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		actionSaver.Commit()
		ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
	})
}
