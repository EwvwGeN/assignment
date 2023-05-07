package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/EwvwGeN/assignment/internal/cache"
	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/restream/reindexer"
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
	cache  *cache.Cache
	db     *reindexer.Reindexer
}

// Creating a connection and launching a cache
func NewServer(config *Config) *Server {
	DbConn := reindexer.NewReindex(
		fmt.Sprintf("cproto://%s:%s/%s", config.DbHost, config.DbPort, config.DBname), reindexer.WithCreateDBIfMissing())
	return &Server{
		router: gin.Default(),
		config: config,
		cache:  cache.NewCache(time.Duration(config.CachelifeTime)*time.Minute, time.Duration(config.CacheCleaningInterval)*time.Minute),
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
