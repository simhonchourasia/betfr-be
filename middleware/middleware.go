package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/authentication"
	"github.com/simhonchourasia/betfr-be/config"
)

var Authentication gin.HandlerFunc = func(c *gin.Context) {
	clientToken := c.Request.Header.Get("token")
	if clientToken == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Missing authorization header"})
		c.Abort()
		return
	}

	claims, err := authentication.ValidateToken(clientToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	c.Set("username", claims.Username)

	c.Next()
}

// https://stackoverflow.com/questions/29418478/go-gin-framework-cors
var CORSMiddleware gin.HandlerFunc = func(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Origin", config.GlobalConfig.OriginFE)
	c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
	c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

	if c.Request.Method == "OPTIONS" {
		c.AbortWithStatus(204)
		return
	}

	c.Next()
}
