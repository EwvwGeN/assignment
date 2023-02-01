package server

import "os"

type Config struct {
	Host           string `yaml:"host"`
	Port           string `yaml:"port"`
	DBname         string `yaml:"dbname"`
	CollectionName string `yaml:"collectionname"`
}

func NewConfig() *Config {
	return &Config{
		Host:           getEnv("HOST", "localhost"),
		Port:           getEnv("PORT", "6534"),
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
