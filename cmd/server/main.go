package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/EwvwGeN/assignment/internal/app/server"
	"github.com/joho/godotenv"
	"github.com/restream/reindexer"
	"gopkg.in/yaml.v2"
)

var (
	isConfig   bool
	configPath string
)

func init() {
	flag.BoolVar(&isConfig, "c", false, "config activation")
	flag.StringVar(&configPath, "config-path", "configs/server.yaml", "path to config file")
}

func main() {
	flag.Parse()
	config := getConfig()
	db := reindexer.NewReindex(fmt.Sprintf("cproto://%s:%s/%s", config.Host, config.Port, config.DBname), reindexer.WithCreateDBIfMissing())
	fmt.Println(db.Ping())
}

func getConfig() *server.Config {
	godotenv.Load()
	config := server.NewConfig()
	if !isConfig {
		return config
	}

	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(file, config)
	if err != nil {
		log.Fatal(err)
	}

	return config
}
