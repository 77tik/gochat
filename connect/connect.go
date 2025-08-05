/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 18:18
 */
package connect

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gochat/config"
	_ "net/http/pprof"
	"runtime"
	"time"
)

var DefaultServer *Server

type Connect struct {
	ServerId string
}

func New() *Connect {
	return new(Connect)
}

func (c *Connect) Run() {
	// 获取Connect层配置
	connectConfig := config.Conf.Connect

	// 设置CPU核心数
	runtime.GOMAXPROCS(connectConfig.ConnectBucket.CpuNum)

	// 居然是初始化logic层客户端，想调用logic层的方法？
	// 是的，调用方法是加入房间和离开房间
	if err := c.InitLogicRpcClient(); err != nil {
		logrus.Panicf("InitLogicRpcClient err:%s", err.Error())
	}
	// logic层居然也会调用本connection层方法，这确实，因为流程图上大概会如此，但是也不太应该，因为logic和connection之间还有消息队列呢？
	Buckets := make([]*Bucket, connectConfig.ConnectBucket.CpuNum)
	for i := 0; i < connectConfig.ConnectBucket.CpuNum; i++ {
		Buckets[i] = NewBucket(BucketOptions{
			ChannelSize:   connectConfig.ConnectBucket.Channel,
			RoomSize:      connectConfig.ConnectBucket.Room,
			RoutineAmount: connectConfig.ConnectBucket.RoutineAmount,
			RoutineSize:   connectConfig.ConnectBucket.RoutineSize,
		})
	}
	// rpc操作符
	// 操作符里面有RpcClient，这个RpcClient会调用 logicRpcClient这个单例的加入房间方法，所以要先初始化logicRpcClient
	operator := new(DefaultOperator)
	DefaultServer = NewServer(Buckets, operator, ServerOptions{
		WriteWait:       10 * time.Second,
		PongWait:        60 * time.Second,
		PingPeriod:      54 * time.Second,
		MaxMessageSize:  512,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		BroadcastSize:   512,
	})
	c.ServerId = fmt.Sprintf("%s-%s", "ws", uuid.New().String())
	//init Connect layer rpc server ,task layer will call this
	// Task 层会调用？
	if err := c.InitConnectWebsocketRpcServer(); err != nil {
		logrus.Panicf("InitConnectWebsocketRpcServer Fatal error: %s \n", err.Error())
	}

	//start Connect layer server handler persistent connection
	// 注册了WS作为路由，那又是谁会调用呢？
	if err := c.InitWebsocket(); err != nil {
		logrus.Panicf("Connect layer InitWebsocket() error:  %s \n", err.Error())
	}
}

func (c *Connect) RunTcp() {
	// get Connect layer config
	connectConfig := config.Conf.Connect

	//set the maximum number of CPUs that can be executing
	runtime.GOMAXPROCS(connectConfig.ConnectBucket.CpuNum)

	//init logic layer rpc client, call logic layer rpc server
	if err := c.InitLogicRpcClient(); err != nil {
		logrus.Panicf("InitLogicRpcClient err:%s", err.Error())
	}
	//init Connect layer rpc server, logic client will call this
	Buckets := make([]*Bucket, connectConfig.ConnectBucket.CpuNum)
	for i := 0; i < connectConfig.ConnectBucket.CpuNum; i++ {
		Buckets[i] = NewBucket(BucketOptions{
			ChannelSize:   connectConfig.ConnectBucket.Channel,
			RoomSize:      connectConfig.ConnectBucket.Room,
			RoutineAmount: connectConfig.ConnectBucket.RoutineAmount,
			RoutineSize:   connectConfig.ConnectBucket.RoutineSize,
		})
	}
	operator := new(DefaultOperator)
	DefaultServer = NewServer(Buckets, operator, ServerOptions{
		WriteWait:       10 * time.Second,
		PongWait:        60 * time.Second,
		PingPeriod:      54 * time.Second,
		MaxMessageSize:  512,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		BroadcastSize:   512,
	})
	//go func() {
	//	http.ListenAndServe("0.0.0.0:9000", nil)
	//}()
	c.ServerId = fmt.Sprintf("%s-%s", "tcp", uuid.New().String())
	//init Connect layer rpc server ,task layer will call this
	if err := c.InitConnectTcpRpcServer(); err != nil {
		logrus.Panicf("InitConnectWebsocketRpcServer Fatal error: %s \n", err.Error())
	}
	//start Connect layer server handler persistent connection by tcp
	if err := c.InitTcpServer(); err != nil {
		logrus.Panicf("Connect layerInitTcpServer() error:%s\n ", err.Error())
	}
}
