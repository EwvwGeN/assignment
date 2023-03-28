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
	NullId           = errors.New("Missing Id")
	DocumentNotExist = errors.New("Document doesnt exist")
	DeplthLevel      = errors.New("Nesting level is higher than allowed")
	InvalidRequest   = errors.New("Invalid request")
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
		simpleDocGroupe.GET("/id=:id", server.getDocById)
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
		server.db.Insert(server.config.CollectionName, &newDocument, "id=serial()")
		ctx.IndentedJSON(http.StatusOK, newDocument)
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

func (server *Server) getDocById(ctx *gin.Context) {
	id := ctx.Param("id")
	doc, found := server.findDoc(id)
	if !found {
		ctx.IndentedJSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("%s: File Id:%s", DocumentNotExist.Error(), id)})
		return
	}
	ctx.IndentedJSON(http.StatusOK, doc)
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
	var parentHeight int
	var maxChildDepth int
	parentId := int64(jsonData["Id"].(float64))
	childs := jsonData["ChildList"].([]interface{})
	hight, err := server.getDocHeight(parentId)
	if err != nil {
		return err
	}
	parentHeight = hight.(int)

	eg := &errgroup.Group{}

	for _, value := range childs {
		id := value
		eg.Go(func() error {
			doc, found := server.findDoc(id)
			if !found {
				return fmt.Errorf("%s: File Id:%d", DocumentNotExist.Error(), id)
			}
			depth := doc.(*models.Document).Depth
			if depth+parentHeight+1 > 2 {
				return fmt.Errorf("%s: File Id:%d", DeplthLevel.Error(), id)
			}
			if depth > maxChildDepth {
				maxChildDepth = depth
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		fmt.Println(err)
		return err
	}
	server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, parentId).
		Set("Depth", maxChildDepth+1).Update()
	for _, value := range childs {
		server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, value).Set("ParentId", parentId).Update()
	}
	return nil
}

func (server *Server) getDocHeight(id interface{}) (interface{}, error) {
	var currentHight int
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
				query := server.db.Query(server.config.CollectionName).Where("id", reindexer.ANY, checkPChild)
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
	fmt.Println(id, " ", doc)
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
