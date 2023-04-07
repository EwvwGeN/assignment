package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/restream/reindexer"
	"golang.org/x/sync/errgroup"
)

var (
	NullId             = errors.New("Missing Id")
	DocumentNotExist   = errors.New("Document doesnt exist")
	DocumentHaveParent = errors.New("Document already have parent")
	DeplthLevel        = errors.New("Nesting level is higher than allowed")
	InvalidRequest     = errors.New("Invalid request")
)

type Server struct {
	router *gin.Engine
	config *Config
	db     *reindexer.Reindexer
}

func NewServer(config *Config) *Server {
	DbConn := reindexer.NewReindex(
		fmt.Sprintf("cproto://%s:%s/%s", config.DbHost, config.DbPort, config.DBname), reindexer.WithCreateDBIfMissing())
	return &Server{
		router: gin.Default(),
		config: config,
		db:     DbConn,
	}
}

func (server *Server) prepareCollections() {
	server.db.OpenNamespace(server.config.CollectionName, reindexer.DefaultNamespaceOptions(), models.Document{})
}

func (server *Server) Start() {
	if err := server.db.Ping(); err != nil {
		panic(err)
	}

	server.prepareCollections()
	server.configureRouter()
	fmt.Println("Server started")
}

func (server *Server) configureRouter() {
	simpleDocGroupe := server.router.Group("/doc")
	{
		simpleDocGroupe.GET("/all", server.getAllDocs())
		simpleDocGroupe.GET("/id=:id", server.getDocById())
		simpleDocGroupe.POST("", server.createDoc())
		simpleDocGroupe.PUT("", server.updateDoc())
		simpleDocGroupe.DELETE("/id=:id", server.deleteDoc())
	}
	bigDocGroupe := server.router.Group("/big-doc")
	{
		bigDocGroupe.GET("/all", server.getAllBigDocs)
	}
	server.router.Run(fmt.Sprintf("%s:%s", server.config.ApiHost, server.config.APiPort))
}

func (server *Server) createDoc() gin.HandlerFunc {
	return server.checkJson(func(ctx *gin.Context) {
		jsonData := ctx.GetStringMap("data")
		jsonStr, _ := json.Marshal(jsonData)
		var newDocument models.Document
		json.Unmarshal(jsonStr, &newDocument)
		childs := []int64{}
		if jsonData["ChildList"] != nil {
			childs = arrAnyToInt64(jsonData["ChildList"].([]interface{}))
			newDocument.ChildList = nil
		}
		if err := server.checkChild(0, childs); err != nil {
			ctx.IndentedJSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		server.db.Insert(server.config.CollectionName, &newDocument, "id=serial()")
		jsonData["Id"] = newDocument.Id
		if err := server.updateChild(jsonData); err != nil {
			ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Errorf("Can not create file: %w", err).Error()})
			return
		}
		doc, _ := server.findDoc(newDocument.Id)
		createdDoc := doc.(*models.Document)
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

func (server *Server) getDocById() gin.HandlerFunc {
	return server.checkExist(func(ctx *gin.Context) {
		id := ctx.GetString("id")
		doc, _ := server.findDoc(id)
		ctx.IndentedJSON(http.StatusOK, doc)
	})
}

func (server *Server) updateDoc() gin.HandlerFunc {
	return server.checkJson(server.checkExist(func(ctx *gin.Context) {
		var document models.AllowedField
		var jsonData map[string]interface{}
		id := ctx.GetString("id")
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

func (server *Server) updateChild(jsonData map[string]interface{}) error {
	if jsonData["ChildList"] == nil {
		return nil
	}
	id := jsonData["Id"].(int64)
	interfaceDoc, _ := server.findDoc(id)
	doc := interfaceDoc.(*models.Document)
	docChilds := doc.ChildList
	inputChilds := arrAnyToInt64(jsonData["ChildList"].([]interface{}))
	delChilds, addChilds := Difference(docChilds, inputChilds)
	if err := server.checkChild(id, addChilds); err != nil {
		return fmt.Errorf("Can not add childs: %w", err)
	}

	firstWg := new(sync.WaitGroup)
	firstWg.Add(len(delChilds))
	for _, childId := range delChilds {
		go func(wg *sync.WaitGroup, childId int64) {
			defer wg.Done()
			server.innerDelete(childId)
		}(firstWg, childId)
		server.innerDelete(childId)
	}
	firstWg.Wait()

	secondWg := new(sync.WaitGroup)
	secondWg.Add(3)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		query := server.db.Query(server.config.CollectionName).WhereInt64("id", reindexer.EQ, inputChilds...)
		query.AggregateMax("Depth")
		iterator := query.Exec()
		maxChildDepth := int(iterator.AggResults()[0].Value)
		iterator.Close()
		currentId := id
		currentDoc := doc
		for i := 1; currentId != 0; i++ {
			if currentDoc.Depth == maxChildDepth+i {
				break
			}
			server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, currentId).
				Set("Depth", maxChildDepth+i).Update()
			buffer, _ := server.findDoc(currentId)
			currentDoc = buffer.(*models.Document)
			currentId = currentDoc.ParentId
		}
	}(secondWg)

	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		server.db.Query(server.config.CollectionName).WhereInt64("id", reindexer.EQ, addChilds...).Set("ParentId", id).Update()
	}(secondWg)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id).Set("ChildList", inputChilds).Update()
	}(secondWg)

	secondWg.Wait()
	return nil
}

func (server *Server) checkChild(id int64, child []int64) error {
	height, err := server.getDocHeight(id)
	if err != nil {
		return err
	}
	docHeight := height.(int)
	eg := &errgroup.Group{}
	for _, value := range child {
		id := value
		eg.Go(func() error {
			doc, found := server.findDoc(id)
			if !found {
				return fmt.Errorf("%s: File Id:%d", DocumentNotExist.Error(), id)
			}
			buffer := doc.(*models.Document)
			parentId := buffer.ParentId
			if parentId != 0 {
				return fmt.Errorf("%s: File Id:%d", DocumentHaveParent.Error(), id)
			}
			depth := buffer.Depth
			if depth+docHeight+1 > server.config.NestingLevel {
				return fmt.Errorf("%s: File Id:%d", DeplthLevel.Error(), id)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}

func (server *Server) getDocHeight(id interface{}) (interface{}, error) {
	var currentHight int
	if id.(int64) == 0 {
		return 0, nil
	}
	doc, found := server.findDoc(id)
	if !found {
		return nil, fmt.Errorf("%s: File Id:%d", DocumentNotExist.Error(), id)
	}
	document := doc.(*models.Document)
	for document.ParentId != 0 {
		currentHight++
		doc, _ = server.findDoc(document.ParentId)
		document = doc.(*models.Document)
	}
	return currentHight, nil
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
			childs, _ := jsonData["ChildList"].([]interface{})
			wg.Add(len(childs))
			for _, value := range childs {
				go func(wg *sync.WaitGroup, id interface{}) {
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
			ParentDoc := interfaceParentDoc.(*models.Document)
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
				checkDoc := bufferCheckDoc.(*models.Document)
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

func (server *Server) innerDelete(id interface{}) *models.Document {
	interfaceDoc, _ := server.findDoc(id)
	doc := interfaceDoc.(*models.Document)
	if doc.ChildList != nil {
		for _, value := range doc.ChildList {
			server.innerDelete(value)
		}
	}
	server.db.Delete(server.config.CollectionName, doc)
	return doc
}

func (server *Server) findDoc(id interface{}) (interface{}, bool) {
	query := server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id)
	doc, found := query.Get()
	return doc, found
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

func (server *Server) bigDoc(input interface{}) models.BigDocument {
	var bigDoc models.BigDocument
	item := input.(*models.Document)
	bigDoc.Id = item.Id
	bigDoc.Body = item.Body
	for _, childId := range item.ChildList {
		childDoc, _ := server.findDoc(childId)
		bigDoc.ChildList = append(bigDoc.ChildList, server.bigDoc(childDoc))
	}
	return bigDoc
}
