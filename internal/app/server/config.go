package server

import "os"

type Config struct {
	ApiHost        string `yaml:"apihost"`
	APiPort        string `yaml:"apiport"`
	DbHost         string `yaml:"host"`
	DbPort         string `yaml:"port"`
	DBname         string `yaml:"dbname"`
	CollectionName string `yaml:"collectionname"`
}

func NewConfig() *Config {
	return &Config{
		ApiHost:        getEnv("API_HOST", "0.0.0.0"),
		APiPort:        getEnv("API_PORT", "8080"),
		DbHost:         getEnv("DB_HOST", "172.18.0.2"),
		DbPort:         getEnv("DB_PORT", "6534"),
		DBname:         getEnv("DB_NAME", "testdb"),
		CollectionName: getEnv("COLLECTION_NAME", "documents"),
	}
}

func getEnv(envKey string, defaultVal string) string {
	if value, exists := os.LookupEnv(envKey); exists {
		return value
	}

	return defaultVal
}
