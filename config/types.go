package config

type Config struct {
	Server ServerDetails `json:"server"`
}

type ServerDetails struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}
