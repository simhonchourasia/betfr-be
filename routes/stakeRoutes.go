package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/controllers"
)

func UnprotectedStakeRoutes(incomingRoutes *gin.Engine) {
}

func ProtectedStakeRoutes(incomingRoutes *gin.Engine) {
	incomingRoutes.POST("/stakes/createstake", controllers.CreateStakeFunc)
}
