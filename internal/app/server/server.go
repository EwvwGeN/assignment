package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/EwvwGeN/assignment/internal/cache"
	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/restream/reindexer/v3"
	_ "github.com/restream/reindexer/v3/bindings/cproto"
)

var (
	NullId             = errors.New("Missing Id")
	DocumentSelfNested = errors.New("Document cant be self nested")
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
		fmt.Sprintf("cproto://%s:%s/%s", config.DbHost, config.DbPort, config.DBname), reindexer.WithCreateDBIfMissing(), reindexer.WithOpenTelemetry())
	return &Server{
		router: gin.Default(),
		config: config,
		cache:  cache.NewCache(time.Duration(config.CachelifeTime)*time.Minute, time.Duration(config.CacheCleaningInterval)*time.Minute),
		db:     DbConn,
	}
}

func (server *Server) prepareCollections() {
	ctx, span := otel.Tracer("Test trace").Start(context.Background(), "rx open ns")
	defer span.End()
	server.db.WithContext(ctx)
	server.db.OpenNamespace(server.config.CollectionName, reindexer.DefaultNamespaceOptions(), models.Document{})
}

func (server *Server) Start() {
	if err := server.db.Ping(); err != nil {
		panic(err)
	}

	exp, _ := stdouttrace.New(
		stdouttrace.WithWriter(os.Stdout),
	)
	resourse, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("assignment"),
			semconv.ServiceVersion("v0.1.0"),
		),
	)
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(resourse),
	)
	defer func() {
		tp.Shutdown(context.Background())
	}()
	otel.SetTracerProvider(tp)

	server.prepareCollections()
	server.configureRouter()
	server.router.Run(fmt.Sprintf("%s:%s", server.config.ApiHost, server.config.APiPort))
}

func (server *Server) configureRouter() {
	simpleDocGroupe := server.router.Group("/docs")
	{
		simpleDocGroupe.GET("", server.getAllDocs())
		simpleDocGroupe.GET("/:id", server.getDocById())
		simpleDocGroupe.POST("", server.createDoc())
		simpleDocGroupe.PUT("", server.updateDoc())
		simpleDocGroupe.DELETE("/:id", server.deleteDoc())
	}
	bigDocGroupe := server.router.Group("/big-docs")
	{
		bigDocGroupe.GET("", server.getAllBigDocs())
		bigDocGroupe.GET("/:id", server.getBigDocById())
	}
}
