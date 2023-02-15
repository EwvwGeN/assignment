package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/restream/reindexer"
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
		simpleDocGroupe.GET("/all", server.getAllDocs)
		simpleDocGroupe.GET("/id=:id", server.getDocById)
		simpleDocGroupe.POST("", server.createDoc)
		simpleDocGroupe.PUT("", server.updateDoc)
		simpleDocGroupe.DELETE("/id=:id", server.deleteDoc)
	}
	bigDocGroupe := server.router.Group("/big-doc")
	{
		bigDocGroupe.GET("/all", server.getAllBigDocs)
	}
	server.router.Run(fmt.Sprintf("%s:%s", server.config.ApiHost, server.config.APiPort))
}

func (server *Server) createDoc(ctx *gin.Context) {
	var newDocument models.Document
	ctx.BindJSON(&newDocument)
	server.db.Upsert(server.config.CollectionName, &newDocument, "id=serial()")
	ctx.IndentedJSON(http.StatusOK, newDocument)
}

func (server *Server) getAllDocs(ctx *gin.Context) {
	query := server.db.Query(server.config.CollectionName)
	iterator := query.Exec()
	defer iterator.Close()
	for iterator.Next() {
		elem := iterator.Object().(*models.Document)
		ctx.IndentedJSON(http.StatusOK, elem)
	}
}

func (server *Server) getDocById(ctx *gin.Context) {
	id := ctx.Param("id")
	doc, found := server.findDoc(id)
	if !found {
		ctx.IndentedJSON(http.StatusNotFound, gin.H{"message": "document doesnt exist"})
		return
	}
	ctx.IndentedJSON(http.StatusOK, doc)
}

func (server *Server) updateDoc(ctx *gin.Context) {
	var document models.AllowedField
	var jsonData map[string]interface{}
	data, _ := ioutil.ReadAll(ctx.Request.Body)
	json.Unmarshal(data, &document)
	json.Unmarshal(data, &jsonData)
	_, found := server.findDoc(document.Id)
	if !found {
		ctx.IndentedJSON(http.StatusNotFound, gin.H{"message": "document doesnt exist"})
		return
	}
	query := server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, document.Id)
	types := reflect.TypeOf(document)
	for key, item := range jsonData {
		if field, exist := types.FieldByName(key); exist {
			query.Set(field.Name, item)
		}
	}
	query.Update()

	ctx.IndentedJSON(http.StatusOK, gin.H{"message": "ok"})
}

func getReindFieldName(field reflect.StructField) string {
	return strings.Split(field.Tag.Get("reindex"), ",")[0]
}

func (server *Server) deleteDoc(ctx *gin.Context) {
	id := ctx.Param("id")
	doc, found := server.findDoc(id)
	if !found {
		ctx.IndentedJSON(http.StatusNotFound, gin.H{"message": "document doesnt exist"})
		return
	}
	server.db.Delete(server.config.CollectionName, doc)
	ctx.IndentedJSON(http.StatusOK, doc)
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
