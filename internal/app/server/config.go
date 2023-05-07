package server

import (
	"os"
	"strconv"
)

type Config struct {
	ApiHost               string `yaml:"api_host"`
	APiPort               string `yaml:"api_port"`
	DbHost                string `yaml:"db_host"`
	DbPort                string `yaml:"db_port"`
	DBname                string `yaml:"db_name"`
	CollectionName        string `yaml:"collection_name"`
	NestingLevel          int    `yaml:"nesting_level"`
	CachelifeTime         int    `yaml:"cache_life_time_min"`
	CacheCleaningInterval int    `yaml:"cache_cleaning_interval_min"`
}

func NewConfig() *Config {
	return &Config{
		ApiHost:               getEnv("API_HOST", "0.0.0.0"),
		APiPort:               getEnv("API_PORT", "8080"),
		DbHost:                getEnv("DB_HOST", "172.18.0.2"),
		DbPort:                getEnv("DB_PORT", "6534"),
		DBname:                getEnv("DB_NAME", "testdb"),
		CollectionName:        getEnv("COLLECTION_NAME", "documents"),
		NestingLevel:          func() int { value, _ := strconv.Atoi(getEnv("NESTING_LEVEL", "2")); return value }(),
		CachelifeTime:         func() int { value, _ := strconv.Atoi(getEnv("CACHE_LIVE_TIME_M", "15")); return value }(),
		CacheCleaningInterval: func() int { value, _ := strconv.Atoi(getEnv("CACHE_CLEANIN_INTERVAL_M", "15")); return value }(),
	}
}

// Retrieves a variable from the environment
func getEnv(envKey string, defaultVal string) string {
	if value, exists := os.LookupEnv(envKey); exists {
		return value
	}

	return defaultVal
}
