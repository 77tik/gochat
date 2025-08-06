/**
 * Created by lock
 * Date: 2019-10-06
 * Time: 23:09
 */
package router

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gochat/api/handler"
	"gochat/api/rpc"
	"gochat/proto"
	"gochat/tools"
	"net/http"
)

func Register() *gin.Engine {
	r := gin.Default()
	// 添加全局跨域中间件
	r.Use(CorsMiddleware())
	// 初始化用户路由
	initUserRouter(r)
	// 初始化推送路由
	initPushRouter(r)

	// 自定义404处理
	r.NoRoute(func(c *gin.Context) {
		tools.FailWithMsg(c, "please check request url !")
	})
	return r
}

func initUserRouter(r *gin.Engine) {
	userGroup := r.Group("/user")
	userGroup.POST("/login", handler.Login)
	userGroup.POST("/register", handler.Register)
	userGroup.Use(CheckSessionId())
	{
		userGroup.POST("/checkAuth", handler.CheckAuth)
		userGroup.POST("/logout", handler.Logout)
	}

}

func initPushRouter(r *gin.Engine) {
	pushGroup := r.Group("/push")
	pushGroup.Use(CheckSessionId())
	{
		pushGroup.POST("/push", handler.Push)
		pushGroup.POST("/pushRoom", handler.PushRoom)
		pushGroup.POST("/count", handler.Count)
		pushGroup.POST("/getRoomInfo", handler.GetRoomInfo)
	}

}

type FormCheckSessionId struct {
	AuthToken string `form:"authToken" json:"authToken" binding:"required"`
}

// 中间件：会话认证
func CheckSessionId() gin.HandlerFunc {
	return func(c *gin.Context) {
		var formCheckSessionId FormCheckSessionId
		if err := c.ShouldBindBodyWith(&formCheckSessionId, binding.JSON); err != nil {
			c.Abort()
			tools.ResponseWithCode(c, tools.CodeSessionError, nil, nil)
			return
		}
		authToken := formCheckSessionId.AuthToken
		req := &proto.CheckAuthRequest{
			AuthToken: authToken,
		}

		// 调logic rpc
		code, userId, userName := rpc.RpcLogicObj.CheckAuth(req)
		if code == tools.CodeFail || userId <= 0 || userName == "" {
			c.Abort()
			tools.ResponseWithCode(c, tools.CodeSessionError, nil, nil)
			return
		}

		// 认证通过，后续处理
		c.Next()
		return
	}
}

// 跨域中间件
func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		var openCorsFlag = true
		if openCorsFlag {
			c.Header("Access-Control-Allow-Origin", "*")                                               // 允许所有源？允许所有域名访问资源，*表示允许所有源
			c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept") // 允许客户端发送额外请求头
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS, POST, PUT, DELETE")                // 允许的HTTP方法
			c.Set("content-type", "application/json")                                                  // 强制设置响应内容为JSON
		}
		if method == "OPTIONS" {
			c.JSON(http.StatusOK, nil)
		}
		c.Next()
	}
}
