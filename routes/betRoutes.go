package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/controllers"
)

func UnprotectedBetRoutes(incomingRoutes *gin.Engine) {
}

func ProtectedBetRoutes(incomingRoutes *gin.Engine) {
	incomingRoutes.POST("/bets/createbetreq", controllers.CreateBetReqFunc)
	incomingRoutes.POST("/bets/handlebetreq", controllers.HandleBetReqFunc)
	incomingRoutes.POST("/bets/resolvebet", controllers.ResolveBetFunc)
}
