package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/controllers"
)

func UnprotectedUserRoutes(incomingRoutes *gin.Engine) {
	incomingRoutes.POST("/users/signup", controllers.SignUpFunc)
	incomingRoutes.POST("/users/login", controllers.LoginFunc)
	incomingRoutes.GET("/users/get", controllers.GetUserFunc)
	incomingRoutes.POST("/users/logout", controllers.LogoutFunc)
	incomingRoutes.DELETE("/users/deleteuser", controllers.DeleteUserFunc)
}

func ProtectedUserRoutes(incomingRoutes *gin.Engine) {
	incomingRoutes.POST("/users/sendfriendreq", controllers.SendFriendReqFunc)
	incomingRoutes.POST("/users/handlefriendreq", controllers.ResolveFriendReqFunc)
}
