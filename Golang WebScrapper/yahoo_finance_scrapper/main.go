package main

import (
	"go-webscraper/middleware"
	"go-webscraper/scraper"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.DebugMode)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-API-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	r.Use(gin.Recovery())

	api := r.Group("/api")
	{
		news := api.Group("/news")
		news.Use(middleware.IPRateLimit())
		{
			news.GET("", scraper.HandleNews)
		}

		stocks := api.Group("/stock")
		stocks.Use(middleware.IPRateLimit())
		{
			stocks.GET("", scraper.HandleStock)
		}
		// Reconsider other Rate Limiter
		sectors := api.Group("/sector")
		sectors.Use(middleware.SectorAPIRateLimit())
		{
			sectors.GET("", scraper.HandleSector)
		}
	}

	if err := r.Run(":8080"); err != nil {
		panic(err)
	}
}
