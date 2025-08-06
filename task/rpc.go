/**
 * Created by lock
 * Date: 2019-08-13
 * Time: 10:13
 */
package task

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rpcxio/libkv/store"
	etcdV3 "github.com/rpcxio/rpcx-etcd/client"
	"github.com/sirupsen/logrus"
	"github.com/smallnest/rpcx/client"
	"gochat/config"
	"gochat/proto"
	"gochat/tools"
	"strings"
	"sync"
	"time"
)

// 这又是沟通谁而做的客户端呢
var RClient = &RpcConnectClient{
	ServerInsMap: make(map[string][]Instance),
	IndexMap:     make(map[string]int),
}

type Instance struct {
	ServerType string
	ServerId   string
	Client     client.XClient
}

type RpcConnectClient struct {
	lock         sync.Mutex
	ServerInsMap map[string][]Instance //serverId--[]ins 为什么有这么多服务实例，每个实例都有一个客户端，我猜测是因为task层要与所有的Connection进行连接？
	IndexMap     map[string]int        //serverId--index 这是干嘛的？
}

// 根据服务实例，使用余数法返回一个节点实例
func (rc *RpcConnectClient) GetRpcClientByServerId(serverId string) (c client.XClient, err error) {
	rc.lock.Lock()
	defer rc.lock.Unlock()
	if _, ok := rc.ServerInsMap[serverId]; !ok || len(rc.ServerInsMap[serverId]) <= 0 {
		return nil, errors.New("no connect layer ip:" + serverId)
	}
	if _, ok := rc.IndexMap[serverId]; !ok {
		rc.IndexMap = map[string]int{
			serverId: 0, // 这是默认的ServerID号吗，这个索引号会用到哪里呢
		}
	}

	// 难道是节点轮询？ 服务索引 % 该服务的实例个数 得到新的索引？，并非新的索引，而是防止溢出
	idx := rc.IndexMap[serverId] % len(rc.ServerInsMap[serverId]) // 卧槽，看上去是多个节点？？

	// 然后取出这个索引的节点实例？
	ins := rc.ServerInsMap[serverId][idx]

	// 轮询+1 有点意思，那么这些节点是怎么放进去的呢？
	rc.IndexMap[serverId] = (rc.IndexMap[serverId] + 1) % len(rc.ServerInsMap[serverId])
	return ins.Client, nil
}

// 返回所有的客户端，所以这个客户端是用来调谁的RPC？
func (rc *RpcConnectClient) GetAllConnectTypeRpcClient() (rpcClientList []client.XClient) {
	for serverId, _ := range rc.ServerInsMap {
		c, err := rc.GetRpcClientByServerId(serverId)
		if err != nil {
			logrus.Infof("GetAllConnectTypeRpcClient err:%s", err.Error())
			continue
		}
		rpcClientList = append(rpcClientList, c)
	}
	return
}

// 原始的形式是什么？先用&划分，然后每个部分再通过 = 划分，如果第一部分是指定的key，就return 第二部分
func getParamByKey(s string, key string) string {
	params := strings.Split(s, "&")
	for _, p := range params {
		kv := strings.Split(p, "=")
		if len(kv) == 2 && kv[0] == key {
			return kv[1]
		}
	}
	return ""
}

func (task *Task) InitConnectRpcClient() (err error) {
	etcdConfigOption := &store.Config{
		ClientTLS:         nil,
		TLS:               nil,
		ConnectionTimeout: time.Duration(config.Conf.Common.CommonEtcd.ConnectionTimeout) * time.Second,
		Bucket:            "",
		PersistConnection: true,
		Username:          config.Conf.Common.CommonEtcd.UserName,
		Password:          config.Conf.Common.CommonEtcd.Password,
	}
	etcdConfig := config.Conf.Common.CommonEtcd
	// 创建etcd服务发现
	d, e := etcdV3.NewEtcdV3Discovery(
		etcdConfig.BasePath,
		etcdConfig.ServerPathConnect,
		[]string{etcdConfig.Host},
		true,
		etcdConfigOption,
	)
	if e != nil {
		logrus.Fatalf("init task rpc etcd discovery client fail:%s", e.Error())
	}
	if len(d.GetServices()) <= 0 {
		logrus.Panicf("no etcd server find!")
	}

	// 遍历所有发现的连接服务
	for _, connectConf := range d.GetServices() {
		logrus.Infof("key is:%s,value is:%s", connectConf.Key, connectConf.Value)
		//RpcConnectClients
		// serverType=XXX2, serverId=XXX2
		serverType := getParamByKey(connectConf.Value, "serverType")
		serverId := getParamByKey(connectConf.Value, "serverId")
		logrus.Infof("serverType is:%s,serverId is:%s", serverType, serverId)
		if serverType == "" || serverId == "" {
			continue
		}

		// 这个key是存什么的？ 是这个吗？tcp@192.168.1.100:8972
		// 点对点服务发现，直接指定服务实例？
		// metadata说是不适用备用发现地址
		d, e := client.NewPeer2PeerDiscovery(connectConf.Key, "")
		if e != nil {
			logrus.Errorf("init task client.NewPeer2PeerDiscovery client fail:%s", e.Error())
			continue
		}
		c := client.NewXClient(
			etcdConfig.ServerPathConnect, // 服务路径，长什么样呢？
			client.Failtry,               // 重试策略
			client.RandomSelect,          // 负载均衡策略?
			d,                            // 服务发现对象，所以是对每一个点对点服务发现专门建一个客户端吗
			client.DefaultOption,         // 客户端配置
		)
		ins := Instance{
			ServerType: serverType,
			ServerId:   serverId,
			Client:     c,
		}
		// 初始化完以后就把实例加到这个全局RClient上
		// 所以etcd上记录的形式是这样吗： tcp@192.168.1.100:8972 : serverType=OOP&serverId=1
		// 所以是修改ServerID来改变Connection层吗？比较ServerID就是Connection
		// 居然不是一个数组吗，我还以为会有很多Connection，目前看来就一个啊
		if _, ok := RClient.ServerInsMap[serverId]; !ok {
			RClient.ServerInsMap[serverId] = []Instance{ins}
		} else {
			RClient.ServerInsMap[serverId] = append(RClient.ServerInsMap[serverId], ins)
		}
	}
	// watch connect server change && update RpcConnectClientList
	// 启动服务变化监听？
	go task.watchServicesChange(d)
	return
}

// 对每个点对点的服务发现进行监控？
// 主体和初始化的逻辑一样，只不过初始化的时候是读取 GetService, 而监控是读取WatchService
func (task *Task) watchServicesChange(d client.ServiceDiscovery) {
	etcdConfig := config.Conf.Common.CommonEtcd
	for kvChan := range d.WatchService() {
		if len(kvChan) <= 0 {
			logrus.Errorf("connect services change, connect alarm, no abailable ip")
		}
		logrus.Infof("connect services change trigger...")
		insMap := make(map[string][]Instance)
		for _, kv := range kvChan {
			logrus.Infof("connect services change,key is:%s,value is:%s", kv.Key, kv.Value)
			serverType := getParamByKey(kv.Value, "serverType")
			serverId := getParamByKey(kv.Value, "serverId")
			logrus.Infof("serverType is:%s,serverId is:%s", serverType, serverId)
			if serverType == "" || serverId == "" {
				continue
			}
			d, e := client.NewPeer2PeerDiscovery(kv.Key, "")
			if e != nil {
				logrus.Errorf("init task client.NewPeer2PeerDiscovery watch client fail:%s", e.Error())
				continue
			}
			c := client.NewXClient(etcdConfig.ServerPathConnect, client.Failtry, client.RandomSelect, d, client.DefaultOption)
			ins := Instance{
				ServerType: serverType,
				ServerId:   serverId,
				Client:     c,
			}
			if _, ok := insMap[serverId]; !ok {
				insMap[serverId] = []Instance{ins}
			} else {
				insMap[serverId] = append(insMap[serverId], ins)
			}
		}
		RClient.lock.Lock()
		RClient.ServerInsMap = insMap
		RClient.lock.Unlock()

	}
}

func (task *Task) pushSingleToConnect(serverId string, userId int, msg []byte) {
	logrus.Infof("pushSingleToConnect Body %s", string(msg))
	pushMsgReq := &proto.PushMsgRequest{
		UserId: userId,
		Msg: proto.Msg{
			Ver:       config.MsgVersion,
			Operation: config.OpSingleSend,
			SeqId:     tools.GetSnowflakeId(),
			Body:      msg,
		},
	}
	reply := &proto.SuccessReply{}
	connectRpc, err := RClient.GetRpcClientByServerId(serverId)
	if err != nil {
		logrus.Infof("get rpc client err %v", err)
	}

	// 调用Connection层的单聊消息发送
	err = connectRpc.Call(context.Background(), "PushSingleMsg", pushMsgReq, reply)
	if err != nil {
		logrus.Infof("pushSingleToConnect Call err %v", err)
	}
	logrus.Infof("reply %s", reply.Msg)
}

// 广播消息发送，话说RPC注册函数进去给人使用，这一块我还没有哦弄清楚？
func (task *Task) broadcastRoomToConnect(roomId int, msg []byte) {
	pushRoomMsgReq := &proto.PushRoomMsgRequest{
		RoomId: roomId,
		Msg: proto.Msg{
			Ver:       config.MsgVersion,
			Operation: config.OpRoomSend,
			SeqId:     tools.GetSnowflakeId(),
			Body:      msg,
		},
	}
	reply := &proto.SuccessReply{}
	rpcList := RClient.GetAllConnectTypeRpcClient()
	for _, rpc := range rpcList {
		logrus.Infof("broadcastRoomToConnect rpc  %v", rpc)
		rpc.Call(context.Background(), "PushRoomMsg", pushRoomMsgReq, reply)
		logrus.Infof("reply %s", reply.Msg)
	}
}

// 广播房间人数
func (task *Task) broadcastRoomCountToConnect(roomId, count int) {
	msg := &proto.RedisRoomCountMsg{
		Count: count,
		Op:    config.OpRoomCountSend,
	}
	var body []byte
	var err error
	if body, err = json.Marshal(msg); err != nil {
		logrus.Warnf("broadcastRoomCountToConnect  json.Marshal err :%s", err.Error())
		return
	}
	pushRoomMsgReq := &proto.PushRoomMsgRequest{
		RoomId: roomId,
		Msg: proto.Msg{
			Ver:       config.MsgVersion,
			Operation: config.OpRoomCountSend,
			SeqId:     tools.GetSnowflakeId(),
			Body:      body,
		},
	}
	reply := &proto.SuccessReply{}
	rpcList := RClient.GetAllConnectTypeRpcClient()
	for _, rpc := range rpcList {
		logrus.Infof("broadcastRoomCountToConnect rpc  %v", rpc)
		rpc.Call(context.Background(), "PushRoomCount", pushRoomMsgReq, reply)
		logrus.Infof("reply %s", reply.Msg)
	}
}

// 广播房间元信息
func (task *Task) broadcastRoomInfoToConnect(roomId int, roomUserInfo map[string]string) {
	msg := &proto.RedisRoomInfo{
		Count:        len(roomUserInfo),
		Op:           config.OpRoomInfoSend,
		RoomUserInfo: roomUserInfo,
		RoomId:       roomId,
	}
	var body []byte
	var err error
	if body, err = json.Marshal(msg); err != nil {
		logrus.Warnf("broadcastRoomInfoToConnect  json.Marshal err :%s", err.Error())
		return
	}
	pushRoomMsgReq := &proto.PushRoomMsgRequest{
		RoomId: roomId,
		Msg: proto.Msg{
			Ver:       config.MsgVersion,
			Operation: config.OpRoomInfoSend,
			SeqId:     tools.GetSnowflakeId(),
			Body:      body,
		},
	}
	reply := &proto.SuccessReply{}
	rpcList := RClient.GetAllConnectTypeRpcClient()
	for _, rpc := range rpcList {
		logrus.Infof("broadcastRoomInfoToConnect rpc  %v", rpc)
		rpc.Call(context.Background(), "PushRoomInfo", pushRoomMsgReq, reply)
		logrus.Infof("broadcastRoomInfoToConnect rpc  reply %v", reply)
	}
}
