/**
 * Created by lock
 * Date: 2019-10-06
 * Time: 23:40
 */
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gochat/api/rpc"
	"gochat/config"
	"gochat/proto"
	"gochat/tools"
	"strconv"
)

// 单聊消息推送
type FormPush struct {
	Msg       string `form:"msg" json:"msg" binding:"required"`
	ToUserId  string `form:"toUserId" json:"toUserId" binding:"required"`
	RoomId    int    `form:"roomId" json:"roomId" binding:"required"`
	AuthToken string `form:"authToken" json:"authToken" binding:"required"`
}

// 单聊消息推送
func Push(c *gin.Context) {
	// 绑定并验证请求参数
	var formPush FormPush

	// 用于将请求数据绑定到结构体上，这不就是和反序列化吗
	if err := c.ShouldBindBodyWith(&formPush, binding.JSON); err != nil {
		tools.FailWithMsg(c, err.Error())
		return
	}
	authToken := formPush.AuthToken
	msg := formPush.Msg
	toUserId := formPush.ToUserId

	// 获取接收者信息
	toUserIdInt, _ := strconv.Atoi(toUserId)
	getUserNameReq := &proto.GetUserInfoRequest{UserId: toUserIdInt}

	// 亏贼，这也能调用logic层的方法啊，获取接收者信息（logic层查库）
	code, toUserName := rpc.RpcLogicObj.GetUserNameByUserId(getUserNameReq)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, "rpc fail get friend userName")
		return
	}

	// 验证发送者身份，掉logic层RPC，token放进去，如果存在就返回元信息，不存在就返回CodeFail了
	checkAuthReq := &proto.CheckAuthRequest{AuthToken: authToken}
	code, fromUserId, fromUserName := rpc.RpcLogicObj.CheckAuth(checkAuthReq)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, "rpc fail get self info")
		return
	}
	roomId := formPush.RoomId

	// 构造推送请求
	req := &proto.Send{
		Msg:          msg,
		FromUserId:   fromUserId,
		FromUserName: fromUserName,
		ToUserId:     toUserIdInt,
		ToUserName:   toUserName,
		RoomId:       roomId,
		Op:           config.OpSingleSend,
	}
	// 调用logic层 把信息发到消息队列中，此处已经和代码逻辑中断了，因为用到了中间件，而task自己也是从中间件消费消息
	code, rpcMsg := rpc.RpcLogicObj.Push(req)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, rpcMsg)
		return
	}
	tools.SuccessWithMsg(c, "ok", nil)
	return
}

// 群聊消息
type FormRoom struct {
	AuthToken string `form:"authToken" json:"authToken" binding:"required"`
	Msg       string `form:"msg" json:"msg" binding:"required"`
	RoomId    int    `form:"roomId" json:"roomId" binding:"required"`
}

func PushRoom(c *gin.Context) {
	var formRoom FormRoom
	// 反序
	if err := c.ShouldBindBodyWith(&formRoom, binding.JSON); err != nil {
		tools.FailWithMsg(c, err.Error())
		return
	}
	authToken := formRoom.AuthToken
	msg := formRoom.Msg
	roomId := formRoom.RoomId

	// 检查认证
	checkAuthReq := &proto.CheckAuthRequest{AuthToken: authToken}
	authCode, fromUserId, fromUserName := rpc.RpcLogicObj.CheckAuth(checkAuthReq)
	if authCode == tools.CodeFail {
		tools.FailWithMsg(c, "rpc fail get self info")
		return
	}
	req := &proto.Send{
		Msg:          msg,
		FromUserId:   fromUserId,
		FromUserName: fromUserName,
		RoomId:       roomId,
		Op:           config.OpRoomSend,
	}

	// 发队列
	code, msg := rpc.RpcLogicObj.PushRoom(req)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, "rpc push room msg fail!")
		return
	}
	tools.SuccessWithMsg(c, "ok", msg)
	return
}

// 房间人数统计：
type FormCount struct {
	RoomId int `form:"roomId" json:"roomId" binding:"required"`
}

// 人数无需验证
func Count(c *gin.Context) {
	var formCount FormCount
	if err := c.ShouldBindBodyWith(&formCount, binding.JSON); err != nil {
		tools.FailWithMsg(c, err.Error())
		return
	}
	roomId := formCount.RoomId
	req := &proto.Send{
		RoomId: roomId,
		Op:     config.OpRoomCountSend,
	}
	code, msg := rpc.RpcLogicObj.Count(req)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, "rpc get room count fail!")
		return
	}
	tools.SuccessWithMsg(c, "ok", msg)
	return
}

type FormRoomInfo struct {
	RoomId int `form:"roomId" json:"roomId" binding:"required"`
}

// 获取房间信息
func GetRoomInfo(c *gin.Context) {
	var formRoomInfo FormRoomInfo
	if err := c.ShouldBindBodyWith(&formRoomInfo, binding.JSON); err != nil {
		tools.FailWithMsg(c, err.Error())
		return
	}
	roomId := formRoomInfo.RoomId
	req := &proto.Send{
		RoomId: roomId,
		Op:     config.OpRoomInfoSend,
	}
	code, msg := rpc.RpcLogicObj.GetRoomInfo(req)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, "rpc get room info fail!")
		return
	}
	tools.SuccessWithMsg(c, "ok", msg)
	return
}
