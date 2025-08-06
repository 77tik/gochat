/**
 * Created by lock
 * Date: 2019-08-12
 * Time: 11:17
 */
package api

import (
	"context"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gochat/api/router"
	"gochat/api/rpc"
	"gochat/config"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Chat struct {
}

func New() *Chat {
	return &Chat{}
}

// api server,Also, you can use gin,echo ... framework wrap
func (c *Chat) Run() {
	//init rpc client
	// 初始化logic层客户端
	rpc.InitLogicRpcClient()

	// gin 引擎注册
	r := router.Register()
	runMode := config.GetGinRunMode()
	logrus.Info("server start , now run mode is ", runMode)

	// os.Getenv("RUN_MODE") 根据env 来指定运行mode
	// 开发模式：提供丰富的调试信息，加速开发迭代
	// 生产模式：最大化性能和安全，确保服务稳定
	// 测试模式：消除日志干扰，便于自动化测试
	gin.SetMode(runMode)
	apiConfig := config.Conf.Api
	port := apiConfig.ApiBase.ListenPort
	flag.Parse()

	// 初始化Server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	go func() {
		// 监听并且提供服务
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Errorf("start listen : %s\n", err)
		}
	}()
	// if have two quit signal , this signal will priority capture ,also can graceful shutdown
	// 创建信号通道
	quit := make(chan os.Signal)

	// 注册信号通知
	signal.Notify(quit,
		syscall.SIGHUP,  // 终端挂起信号
		syscall.SIGINT,  // 中断信号 ctrl + C
		syscall.SIGTERM, // 终止信号 kill
		syscall.SIGQUIT, // 退出信号 ctrl + \
	)

	// 阻塞等待信号
	<-quit
	logrus.Infof("Shutdown Server ...")

	// 我勒个优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logrus.Errorf("Server Shutdown:", err)
	}
	logrus.Infof("Server exiting")
	os.Exit(0)
}
