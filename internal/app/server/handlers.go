package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
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
		if jsonData["ChildList"] != nil {
			childs = util.ArrToInt64(jsonData["ChildList"].([]interface{}))
			newDocument.ChildList = nil
		}
		if err := server.checkChild(0, childs); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("Can not create file: Can not add childs: %w", err).Error()})
			return
		}
		server.db.Insert(server.config.CollectionName, &newDocument, "id=serial()")
		jsonData["Id"] = newDocument.Id
		if err := server.updateChild(jsonData); err != nil {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		doc, _ := server.findDoc(newDocument.Id)
		createdDoc := doc
		ctx.IndentedJSON(http.StatusOK, createdDoc)
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
		fmt.Println("handler", id)
		doc, _ := server.findDoc(id)
		ctx.IndentedJSON(http.StatusOK, doc)
	})
}

func (server *Server) updateDoc() gin.HandlerFunc {
	return server.checkJson(server.checkExist(func(ctx *gin.Context) {
		var document models.AllowedField
		var jsonData map[string]interface{}
		id := ctx.GetInt64("id")
		jsonData = ctx.GetStringMap("data")

		if err := server.updateChild(jsonData); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		query := server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id)
		types := reflect.TypeOf(document)
		for key, value := range jsonData {
			if field, exist := types.FieldByName(key); exist {
				query.Set(field.Name, value)
			}
		}
		query.Update()

		ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
	}))
}

func (server *Server) deleteDoc() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		var jsonData map[string]interface{}
		id, _ := strconv.ParseInt(ctx.GetString("id"), 10, 64)
		jsonData = ctx.GetStringMap("data")
		upperWg := new(sync.WaitGroup)
		upperWg.Add(2)
		go func(upperWg *sync.WaitGroup) {
			defer upperWg.Done()
			wg := new(sync.WaitGroup)
			buffer, _ := jsonData["ChildList"].([]interface{})
			childs := util.ArrToInt64(buffer)
			wg.Add(len(childs))
			for _, value := range childs {
				go func(wg *sync.WaitGroup, id int64) {
					defer wg.Done()
					server.innerDelete(id)
				}(wg, value)
			}
			wg.Wait()
		}(upperWg)

		go func(upperWg *sync.WaitGroup) {
			defer upperWg.Done()
			parentId := int64(jsonData["ParentId"].(float64))
			if parentId == 0 {
				server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id).Delete()
				return
			}
			interfaceParentDoc, _ := server.findDoc(parentId)
			ParentDoc := interfaceParentDoc
			parentChild := func() []int64 {
				buffer := ParentDoc.ChildList
				for i, value := range buffer {
					if value == id {
						return append(buffer[:i], buffer[i+1:]...)
					}
				}
				return nil
			}()
			checkPId := parentId
			checkPChild := parentChild
			checkPDepth := ParentDoc.Depth
			for checkPId != 0 {
				query := server.db.Query(server.config.CollectionName).WhereInt64("id", reindexer.EQ, checkPChild...)
				query.AggregateMax("Depth")
				iterator := query.Exec()
				maxChildDepth := int(iterator.AggResults()[0].Value)
				iterator.Close()
				if maxChildDepth+1 == checkPDepth {
					break
				}
				server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, checkPId).
					Set("Depth", maxChildDepth+1).Update()
				bufferCheckDoc, _ := server.findDoc(checkPId)
				checkDoc := bufferCheckDoc
				checkPId = checkDoc.ParentId
				checkPChild = checkDoc.ChildList
				checkPDepth = checkDoc.Depth
			}

			server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, parentId).
				Set("ChildList", parentChild).Update()
		}(upperWg)

		upperWg.Wait()
		ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
	})
}
