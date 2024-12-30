package config

type Config struct {
	Server             ServerDetails `json:"server"`
	ConnectIntervalSec int           `json:"connect_interval_sec"`
}

type ServerDetails struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}
