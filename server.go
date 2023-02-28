package main

import (
	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/config"
	"github.com/simhonchourasia/betfr-be/middleware"
	"github.com/simhonchourasia/betfr-be/routes"
)

// testing
func main() {
	err := config.SetupConfig()
	if err != nil {
		panic("Error in config: " + err.Error())
	}

	port := config.GlobalConfig.Port

	router := gin.New()
	// TODO: specify trusted proxies
	router.Use(gin.Logger())
	routes.UnprotectedUserRoutes(router) // Signup and login
	routes.UnprotectedBetRoutes(router)
	routes.UnprotectedStakeRoutes(router)

	router.Use(middleware.Authentication)
	routes.ProtectedUserRoutes(router)
	routes.ProtectedBetRoutes(router)
	routes.ProtectedStakeRoutes(router)

	// API-2
	router.GET("/api-1", func(c *gin.Context) {

		c.JSON(200, gin.H{"success": "Access granted for api-1"})

	})

	// API-1
	router.GET("/api-2", func(c *gin.Context) {
		c.JSON(200, gin.H{"success": "Access granted for api-2"})
	})

	router.Run(":" + port)
}
