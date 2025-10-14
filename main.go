package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db  *gorm.DB
	rdb *redis.Client
	ctx = context.Background()
)

type User struct {
	ID    uint   `gorm:"primaryKey;autoIncrement"`
	Name  string `json:"name"`
	Email string `json:"email" gorm:"unique"`
}

func initDB() *gorm.DB {
	dsn := os.Getenv("POSTGRES_DSN")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	db.AutoMigrate(&User{})
	return db
}

func initRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatal("Failed to connect with redis %v", err)
	}
	return rdb
}

func getAllUsers(c echo.Context) error {
	var users []User
	if err := db.Find(&users).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "DB ERROR"})
	}

	// Cache the full list in Redis
	data, _ := json.Marshal(users)
	rdb.Set(ctx, "all_users", data, 10*time.Minute)

	return c.JSON(http.StatusOK, users)
}

func createUser(c echo.Context) error {
	u := new(User)
	if err := c.Bind(u); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid Request"})
	}
	var lastUser User
	if err := db.Order("id desc").First(&lastUser).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "DB ERROR"})
	}
	u.ID = lastUser.ID + 1

	// Insert into db

	if err := db.Create(&u).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "DB ERROR"})
	}
	var users []User
	if err := db.Find(&users).Error; err == nil {
		data, _ := json.Marshal(users)
		rdb.Set(ctx, "all_users", data, 10*time.Minute)
	}
	// Also cache individual user

	data, _ := json.Marshal(u)
	rdb.Set(ctx, fmt.Sprintf("user:%d", u.ID), data, 10*time.Minute)
	return c.JSON(http.StatusCreated, u)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	db = initDB()
	rdb = initRedis()
	e := echo.New()

	e.GET("/allUsers", getAllUsers)
	e.POST("/user", createUser)

	e.Logger.Fatal(e.Start(":8080"))
}
