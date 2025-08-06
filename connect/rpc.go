/**
 * Created by lock
 * Date: 2019-08-12
 * Time: 23:36
 */
package connect

import (
	"context"
	"errors"
	"fmt"
	"github.com/rcrowley/go-metrics"
	"github.com/rpcxio/libkv/store"
	etcdV3 "github.com/rpcxio/rpcx-etcd/client"
	"github.com/rpcxio/rpcx-etcd/serverplugin"
	"github.com/sirupsen/logrus"
	"github.com/smallnest/rpcx/client"
	"github.com/smallnest/rpcx/server"
	"gochat/config"
	"gochat/proto"
	"gochat/tools"
	"strings"
	"sync"
	"time"
)

// 沟通logic层的客户端，单例模式
var logicRpcClient client.XClient
var once sync.Once

type RpcConnect struct {
}

// 初始化logic层客户端，为什么需要etcd呢？
// 因为要调用logic层注册在etcd中的服务
func (c *Connect) InitLogicRpcClient() (err error) {
	// 初始化etcd配置
	etcdConfigOption := &store.Config{
		ClientTLS:         nil,
		TLS:               nil,
		ConnectionTimeout: time.Duration(config.Conf.Common.CommonEtcd.ConnectionTimeout) * time.Second,
		Bucket:            "",
		PersistConnection: true,
		Username:          config.Conf.Common.CommonEtcd.UserName,
		Password:          config.Conf.Common.CommonEtcd.Password,
	}
	once.Do(func() {
		// 创建etcd服务发现
		d, e := etcdV3.NewEtcdV3Discovery(
			config.Conf.Common.CommonEtcd.BasePath,
			config.Conf.Common.CommonEtcd.ServerPathLogic,
			[]string{config.Conf.Common.CommonEtcd.Host},
			true,
			etcdConfigOption,
		)
		if e != nil {
			logrus.Fatalf("init connect rpc etcd discovery client fail:%s", e.Error())
		}
		// 创建RPC客户端
		logicRpcClient = client.NewXClient(
			config.Conf.Common.CommonEtcd.ServerPathLogic, // 服务发现路径
			client.Failtry,      // 失败重试策略
			client.RandomSelect, // 随机负载均衡
			d,
			client.DefaultOption,
		)
	})
	if logicRpcClient == nil {
		return errors.New("get rpc client nil")
	}
	return
}

// 加入房间（rpc调用logic层connect方法，logic初始化时已注册进etcd）
func (rpc *RpcConnect) Connect(connReq *proto.ConnectRequest) (uid int, err error) {
	reply := &proto.ConnectReply{}

	// 调用logic层的Connect方法，其实就是加入房间
	err = logicRpcClient.Call(context.Background(), "Connect", connReq, reply)
	if err != nil {
		logrus.Fatalf("failed to call: %v", err)
	}
	uid = reply.UserId
	logrus.Infof("connect logic userId :%d", reply.UserId)
	return
}

// 离开房间（rpc调用logic层disconnect方法，logic初始化时已注册进etcd）
func (rpc *RpcConnect) DisConnect(disConnReq *proto.DisConnectRequest) (err error) {
	reply := &proto.DisConnectReply{}
	if err = logicRpcClient.Call(context.Background(), "DisConnect", disConnReq, reply); err != nil {
		logrus.Fatalf("failed to call: %v", err)
	}
	return
}

// 注册ws rpc Server，其实流程和logic层注册差不多，都是读地址，然后每个地址都启动server服务
// 但是这又是给谁调用的？是API层吗？
func (c *Connect) InitConnectWebsocketRpcServer() (err error) {
	var network, addr string
	connectRpcAddress := strings.Split(config.Conf.Connect.ConnectRpcAddressWebSockts.Address, ",")
	for _, bind := range connectRpcAddress {
		if network, addr, err = tools.ParseNetwork(bind); err != nil {
			logrus.Panicf("InitConnectWebsocketRpcServer ParseNetwork error : %s", err)
		}
		logrus.Infof("Connect start run at-->%s:%s", network, addr)
		go c.createConnectWebsocktsRpcServer(network, addr)
	}
	return
}

// 初始化tcp rpc Server
func (c *Connect) InitConnectTcpRpcServer() (err error) {
	var network, addr string
	connectRpcAddress := strings.Split(config.Conf.Connect.ConnectRpcAddressTcp.Address, ",")
	for _, bind := range connectRpcAddress {
		if network, addr, err = tools.ParseNetwork(bind); err != nil {
			logrus.Panicf("InitConnectTcpRpcServer ParseNetwork error : %s", err)
		}
		logrus.Infof("Connect start run at-->%s:%s", network, addr)
		go c.createConnectTcpRpcServer(network, addr)
	}
	return
}

// 消息推送载体
type RpcConnectPush struct {
}

// 单聊消息推送
func (rpc *RpcConnectPush) PushSingleMsg(ctx context.Context, pushMsgReq *proto.PushMsgRequest, successReply *proto.SuccessReply) (err error) {
	var (
		bucket  *Bucket
		channel *Channel
	)
	logrus.Info("rpc PushMsg :%v ", pushMsgReq)
	if pushMsgReq == nil {
		logrus.Errorf("rpc PushSingleMsg() args:(%v)", pushMsgReq)
		return
	}
	// 通过服务器找到筒子，通过筒子找到对应的节点Channel，然后推
	bucket = DefaultServer.Bucket(pushMsgReq.UserId)
	if channel = bucket.Channel(pushMsgReq.UserId); channel != nil {
		err = channel.Push(&pushMsgReq.Msg)
		logrus.Infof("DefaultServer Channel err nil ,args: %v", pushMsgReq)
		return
	}
	successReply.Code = config.SuccessReplyCode
	successReply.Msg = config.SuccessReplyMsg
	logrus.Infof("successReply:%v", successReply)
	return
}

// 群聊消息推送
func (rpc *RpcConnectPush) PushRoomMsg(ctx context.Context, pushRoomMsgReq *proto.PushRoomMsgRequest, successReply *proto.SuccessReply) (err error) {
	successReply.Code = config.SuccessReplyCode
	successReply.Msg = config.SuccessReplyMsg
	logrus.Infof("PushRoomMsg msg %+v", pushRoomMsgReq)
	for _, bucket := range DefaultServer.Buckets {
		bucket.BroadcastRoom(pushRoomMsgReq)
	}
	return
}

// 怎么和上面的是一样的实现
func (rpc *RpcConnectPush) PushRoomCount(ctx context.Context, pushRoomMsgReq *proto.PushRoomMsgRequest, successReply *proto.SuccessReply) (err error) {
	successReply.Code = config.SuccessReplyCode
	successReply.Msg = config.SuccessReplyMsg
	logrus.Infof("PushRoomCount msg %v", pushRoomMsgReq)
	for _, bucket := range DefaultServer.Buckets {
		bucket.BroadcastRoom(pushRoomMsgReq)
	}
	return
}

func (rpc *RpcConnectPush) PushRoomInfo(ctx context.Context, pushRoomMsgReq *proto.PushRoomMsgRequest, successReply *proto.SuccessReply) (err error) {
	successReply.Code = config.SuccessReplyCode
	successReply.Msg = config.SuccessReplyMsg
	logrus.Infof("connect,PushRoomInfo msg %+v", pushRoomMsgReq)
	for _, bucket := range DefaultServer.Buckets {
		bucket.BroadcastRoom(pushRoomMsgReq)
	}
	return
}

// 与logic层一样的注册服务，启动Server
func (c *Connect) createConnectWebsocktsRpcServer(network string, addr string) {
	s := server.NewServer()
	addRegistryPlugin(s, network, addr)
	//config.Conf.Connect.ConnectTcp.ServerId
	//s.RegisterName(config.Conf.Common.CommonEtcd.ServerPathConnect, new(RpcConnectPush), fmt.Sprintf("%s", config.Conf.Connect.ConnectWebsocket.ServerId))
	s.RegisterName(config.Conf.Common.CommonEtcd.ServerPathConnect, new(RpcConnectPush), fmt.Sprintf("serverId=%s&serverType=ws", c.ServerId))
	s.RegisterOnShutdown(func(s *server.Server) {
		s.UnregisterAll()
	})
	s.Serve(network, addr)
}

func (c *Connect) createConnectTcpRpcServer(network string, addr string) {
	s := server.NewServer()
	addRegistryPlugin(s, network, addr)
	//s.RegisterName(config.Conf.Common.CommonEtcd.ServerPathConnect, new(RpcConnectPush), fmt.Sprintf("%s", config.Conf.Connect.ConnectTcp.ServerId))

	// 这应该是注册方法，方法都放在结构体上，所以把结构体方法都注册进去
	s.RegisterName(config.Conf.Common.CommonEtcd.ServerPathConnect, new(RpcConnectPush), fmt.Sprintf("serverId=%s&serverType=tcp", c.ServerId))
	s.RegisterOnShutdown(func(s *server.Server) {
		s.UnregisterAll()
	})
	s.Serve(network, addr)
}

// 这应该是注册路径
func addRegistryPlugin(s *server.Server, network string, addr string) {
	r := &serverplugin.EtcdV3RegisterPlugin{
		ServiceAddress: network + "@" + addr,
		EtcdServers:    []string{config.Conf.Common.CommonEtcd.Host},
		BasePath:       config.Conf.Common.CommonEtcd.BasePath,
		Metrics:        metrics.NewRegistry(),
		UpdateInterval: time.Minute,
	}
	err := r.Start()
	if err != nil {
		logrus.Fatal(err)
	}
	s.Plugins.Add(r)
}
