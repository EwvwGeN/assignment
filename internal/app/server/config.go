package server

type Config struct {
	Host   string `yaml:"host"`
	Port   string `yaml:"port"`
	DBname string `yaml:"dbname"`
}

func NewConfig() *Config {
	return &Config{
		Host:   "localhost",
		Port:   "6534",
		DBname: "testdb",
	}
}
