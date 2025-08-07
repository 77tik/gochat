/**
 * Created by lock
 * Date: 2019-08-12
 * Time: 15:52
 */
package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"gochat/logic/dao"
	"gochat/proto"
	"gochat/tools"
	"strconv"
	"time"
)

type RpcLogic struct {
}

func (rpc *RpcLogic) Register(ctx context.Context, args *proto.RegisterRequest, reply *proto.RegisterReply) (err error) {
	reply.Code = config.FailReplyCode
	u := new(dao.User)
	uData := u.CheckHaveUserName(args.Name)
	if uData.Id > 0 {
		return errors.New("this user name already have , please login !!!")
	}
	u.UserName = args.Name
	u.Password = args.Password
	userId, err := u.Add()
	if err != nil {
		logrus.Infof("register err:%s", err.Error())
		return err
	}
	if userId == 0 {
		return errors.New("register userId empty!")
	}
	//set token 生成会话令牌
	randToken := tools.GetRandomToken(32)
	sessionId := tools.CreateSessionId(randToken)
	userData := make(map[string]interface{})
	userData["userId"] = userId
	userData["userName"] = args.Name
	RedisSessClient.Do("MULTI")
	RedisSessClient.HMSet(sessionId, userData)
	RedisSessClient.Expire(sessionId, 86400*time.Second)
	err = RedisSessClient.Do("EXEC").Err()
	if err != nil {
		logrus.Infof("register set redis token fail!")
		return err
	}

	// 操作成功，返回认证令牌
	reply.Code = config.SuccessReplyCode
	reply.AuthToken = randToken
	return
}

// 登录
func (rpc *RpcLogic) Login(ctx context.Context, args *proto.LoginRequest, reply *proto.LoginResponse) (err error) {

	// 先初始化返回状态
	reply.Code = config.FailReplyCode
	u := new(dao.User)
	userName := args.Name
	passWord := args.Password

	// 检查用户状态
	data := u.CheckHaveUserName(userName)
	if (data.Id == 0) || (passWord != data.Password) {
		return errors.New("no this user or password error!")
	}

	// 创建登录会话ID
	loginSessionId := tools.GetSessionIdByUserId(data.Id)
	//set token
	//err = redis.HMSet(auth, userData)
	randToken := tools.GetRandomToken(32)
	sessionId := tools.CreateSessionId(randToken)
	userData := make(map[string]interface{})
	userData["userId"] = data.Id
	userData["userName"] = data.UserName
	//check is login，根据登录会话ID查一下是否登录
	token, _ := RedisSessClient.Get(loginSessionId).Result()

	// 如果登陆了就删掉，相当于刷新一下登录状态，删了redis中缓存再添加
	if token != "" {
		//logout already login user session
		oldSession := tools.CreateSessionId(token)
		err := RedisSessClient.Del(oldSession).Err()
		if err != nil {
			return errors.New("logout user fail!token is:" + token)
		}
	}
	RedisSessClient.Do("MULTI")
	RedisSessClient.HMSet(sessionId, userData)
	RedisSessClient.Expire(sessionId, 86400*time.Second)
	RedisSessClient.Set(loginSessionId, randToken, 86400*time.Second)
	err = RedisSessClient.Do("EXEC").Err()
	//err = RedisSessClient.Set(authToken, data.Id, 86400*time.Second).Err()
	if err != nil {
		logrus.Infof("register set redis token fail!")
		return err
	}

	// 返回登录成功状态码以及token
	reply.Code = config.SuccessReplyCode
	reply.AuthToken = randToken
	return
}

// 获取user信息：id和name
func (rpc *RpcLogic) GetUserInfoByUserId(ctx context.Context, args *proto.GetUserInfoRequest, reply *proto.GetUserInfoResponse) (err error) {
	reply.Code = config.FailReplyCode
	userId := args.UserId
	u := new(dao.User)
	userName := u.GetUserNameByUserId(userId)
	reply.UserId = userId
	reply.UserName = userName
	reply.Code = config.SuccessReplyCode
	return
}

func (rpc *RpcLogic) CheckAuth(ctx context.Context, args *proto.CheckAuthRequest, reply *proto.CheckAuthResponse) (err error) {
	reply.Code = config.FailReplyCode
	authToken := args.AuthToken

	// 通过登录认证令牌拿到会话名
	sessionName := tools.GetSessionName(authToken)
	var userDataMap = map[string]string{}
	userDataMap, err = RedisSessClient.HGetAll(sessionName).Result()
	if err != nil {
		logrus.Infof("check auth fail!,authToken is:%s", authToken)
		return err
	}
	if len(userDataMap) == 0 {
		logrus.Infof("no this user session,authToken is:%s", authToken)
		return
	}

	// 如果有会话存在就填充用户元信息，否则就像上面那样返回err
	intUserId, _ := strconv.Atoi(userDataMap["userId"])
	reply.UserId = intUserId
	userName, _ := userDataMap["userName"]
	reply.Code = config.SuccessReplyCode
	reply.UserName = userName
	return
}

// 登出
func (rpc *RpcLogic) Logout(ctx context.Context, args *proto.LogoutRequest, reply *proto.LogoutResponse) (err error) {
	reply.Code = config.FailReplyCode
	authToken := args.AuthToken
	sessionName := tools.GetSessionName(authToken)

	var userDataMap = map[string]string{}
	userDataMap, err = RedisSessClient.HGetAll(sessionName).Result()
	if err != nil {
		logrus.Infof("check auth fail!,authToken is:%s", authToken)
		return err
	}
	if len(userDataMap) == 0 {
		logrus.Infof("no this user session,authToken is:%s", authToken)
		return
	}

	// 先验证会话，如果不存在那就结束了，皆大欢喜，存在的话就要一个一个删了
	// 并非是验证
	intUserId, _ := strconv.Atoi(userDataMap["userId"])
	sessIdMap := tools.GetSessionIdByUserId(intUserId)
	//del sess_map like sess_map_1
	err = RedisSessClient.Del(sessIdMap).Err()
	if err != nil {
		logrus.Infof("logout del sess map error:%s", err.Error())
		return err
	}
	//del serverId 为什么？
	// 删除用户当前链接的Connect层服务器标识，因为用户已经登出了，所以这个链接就要删掉，具体的逻辑，还是到connect层再看
	logic := new(Logic)
	serverIdKey := logic.getUserKey(fmt.Sprintf("%d", intUserId))
	err = RedisSessClient.Del(serverIdKey).Err()
	if err != nil {
		logrus.Infof("logout del server id error:%s", err.Error())
		return err
	}
	err = RedisSessClient.Del(sessionName).Err()
	if err != nil {
		logrus.Infof("logout error:%s", err.Error())
		return err
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
single send msg 发消息队列中
*/
func (rpc *RpcLogic) Push(ctx context.Context, args *proto.Send, reply *proto.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	sendData := args
	var bodyBytes []byte
	bodyBytes, err = json.Marshal(sendData)
	if err != nil {
		logrus.Errorf("logic,push msg fail,err:%s", err.Error())
		return
	}

	// 获取接收者所在的Connection服务器层，这个存在redis中
	logic := new(Logic)
	userSidKey := logic.getUserKey(fmt.Sprintf("%d", sendData.ToUserId))
	serverIdStr := RedisSessClient.Get(userSidKey).Val()
	//var serverIdInt int
	//serverIdInt, err = strconv.Atoi(serverId)
	if err != nil {
		logrus.Errorf("logic,push parse int fail:%s", err.Error())
		return
	}

	// 推送到对应的队列中
	err = logic.RedisPublishChannel(serverIdStr, sendData.ToUserId, bodyBytes)
	if err != nil {
		logrus.Errorf("logic,redis publish err: %s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
push msg to room 群聊消息推送 到队列中
*/
func (rpc *RpcLogic) PushRoom(ctx context.Context, args *proto.Send, reply *proto.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	sendData := args
	roomId := sendData.RoomId
	logic := new(Logic)
	roomUserInfo := make(map[string]string)
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(roomId))
	roomUserInfo, err = RedisClient.HGetAll(roomUserKey).Result()
	if err != nil {
		logrus.Errorf("logic,PushRoom redis hGetAll err:%s", err.Error())
		return
	}
	//if len(roomUserInfo) == 0 {
	//	return errors.New("no this user")
	//}
	var bodyBytes []byte
	sendData.RoomId = roomId
	sendData.Msg = args.Msg
	sendData.FromUserId = args.FromUserId
	sendData.FromUserName = args.FromUserName
	sendData.Op = config.OpRoomSend
	sendData.CreateTime = tools.GetNowDateTime()
	bodyBytes, err = json.Marshal(sendData)
	if err != nil {
		logrus.Errorf("logic,PushRoom Marshal err:%s", err.Error())
		return
	}
	err = logic.RedisPublishRoomInfo(roomId, len(roomUserInfo), roomUserInfo, bodyBytes)
	if err != nil {
		logrus.Errorf("logic,PushRoom err:%s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
get room online person count 获取房间在线人数信息
*/
func (rpc *RpcLogic) Count(ctx context.Context, args *proto.Send, reply *proto.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	roomId := args.RoomId
	logic := new(Logic)
	var count int
	count, err = RedisSessClient.Get(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", roomId))).Int()
	err = logic.RedisPushRoomCount(roomId, count)
	if err != nil {
		logrus.Errorf("logic,Count err:%s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
get room info
*/
func (rpc *RpcLogic) GetRoomInfo(ctx context.Context, args *proto.Send, reply *proto.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	logic := new(Logic)
	roomId := args.RoomId
	roomUserInfo := make(map[string]string)
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(roomId))
	roomUserInfo, err = RedisClient.HGetAll(roomUserKey).Result()
	if len(roomUserInfo) == 0 {
		return errors.New("getRoomInfo no this user")
	}
	err = logic.RedisPushRoomInfo(roomId, len(roomUserInfo), roomUserInfo)
	if err != nil {
		logrus.Errorf("logic,GetRoomInfo err:%s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

// 加入房间
func (rpc *RpcLogic) Connect(ctx context.Context, args *proto.ConnectRequest, reply *proto.ConnectReply) (err error) {
	if args == nil {
		logrus.Errorf("logic,connect args empty")
		return
	}

	// 验证会话
	logic := new(Logic)
	//key := logic.getUserKey(args.AuthToken)
	logrus.Infof("logic,authToken is:%s", args.AuthToken)
	key := tools.GetSessionName(args.AuthToken)
	userInfo, err := RedisClient.HGetAll(key).Result()
	if err != nil {
		logrus.Infof("RedisCli HGetAll key :%s , err:%s", key, err.Error())
		return err
	}
	if len(userInfo) == 0 {
		reply.UserId = 0
		return
	}
	reply.UserId, _ = strconv.Atoi(userInfo["userId"])
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(args.RoomId))
	if reply.UserId != 0 {
		userKey := logic.getUserKey(fmt.Sprintf("%d", reply.UserId))
		logrus.Infof("logic redis set userKey:%s, serverId : %s", userKey, args.ServerId)
		validTime := config.RedisBaseValidTime * time.Second

		// 记录用户 - 服务器 映射
		err = RedisClient.Set(userKey, args.ServerId, validTime).Err()
		if err != nil {
			logrus.Warnf("logic set err:%s", err)
		}

		// 加入房间，人数加1，房间记录新用户
		if RedisClient.HGet(roomUserKey, fmt.Sprintf("%d", reply.UserId)).Val() == "" {
			RedisClient.HSet(roomUserKey, fmt.Sprintf("%d", reply.UserId), userInfo["userName"])
			// add room user count ++
			RedisClient.Incr(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", args.RoomId)))
		}
	}
	logrus.Infof("logic rpc userId:%d", reply.UserId)
	return
}

// 离开房间
func (rpc *RpcLogic) DisConnect(ctx context.Context, args *proto.DisConnectRequest, reply *proto.DisConnectReply) (err error) {
	logic := new(Logic)
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(args.RoomId))
	// room user count -- 更新在线人数
	if args.RoomId > 0 {
		count, _ := RedisSessClient.Get(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", args.RoomId))).Int()
		if count > 0 {
			RedisClient.Decr(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", args.RoomId))).Result()
		}
	}
	// room login user-- 将用户从map中移除
	if args.UserId != 0 {
		err = RedisClient.HDel(roomUserKey, fmt.Sprintf("%d", args.UserId)).Err()
		if err != nil {
			logrus.Warnf("HDel getRoomUserKey err : %s", err)
		}
	}
	//below code can optimize send a signal to queue,another process get a signal from queue,then push event to websocket
	// 下方代码可优化为：发送信号到队列，再由另一个进程从队列获取信号并推送事件到WebSocket
	// 但是我看来这其实就已经推送到队列中，这个logic层的逻辑从来不自己处理逻辑，都是推送到队列中
	roomUserInfo, err := RedisClient.HGetAll(roomUserKey).Result()
	if err != nil {
		logrus.Warnf("RedisCli HGetAll roomUserInfo key:%s, err: %s", roomUserKey, err)
	}
	if err = logic.RedisPublishRoomInfo(args.RoomId, len(roomUserInfo), roomUserInfo, nil); err != nil {
		logrus.Warnf("publish RedisPublishRoomCount err: %s", err.Error())
		return
	}
	return
}
