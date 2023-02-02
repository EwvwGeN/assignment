package server

import (
	"fmt"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/restream/reindexer"
)

type Server struct {
	config *Config
	db     *reindexer.Reindexer
}

func NewServer(config *Config) *Server {
	DbConn := reindexer.NewReindex(
		fmt.Sprintf("cproto://%s:%s/%s", config.Host, config.Port, config.DBname), reindexer.WithCreateDBIfMissing())
	return &Server{
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
	fmt.Println("Server started")
}
