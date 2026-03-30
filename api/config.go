package main

import (
	"os"
	"strconv"
)

type Config struct {
	TCPPort       int
	HTTPPort      int
	DatabaseURL   string
	RedisURL      string
	JWTSecret     string
	AdminUsername string
	AdminPassword string
}

func LoadConfig() Config {
	return Config{
		TCPPort:       getEnvInt("TCP_PORT", 9999),
		HTTPPort:      getEnvInt("HTTP_PORT", 5000),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		AdminUsername: os.Getenv("ADMIN_USERNAME"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}
