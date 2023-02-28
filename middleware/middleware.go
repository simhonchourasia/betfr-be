package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/authentication"
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
