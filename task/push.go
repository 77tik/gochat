/**
 * Created by lock
 * Date: 2019-08-13
 * Time: 10:50
 */
package task

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"gochat/proto"
	"math/rand"
)

type PushParams struct {
	ServerId string // 这是推动对应的connection处理？
	UserId   int    // 这是发送者，还是接收者？
	Msg      []byte
	RoomId   int
}

// 推送参数放入管道？难道推送参数的数量，就代表消息的个数？
var pushChannel []chan *PushParams

func init() {
	pushChannel = make([]chan *PushParams, config.Conf.Task.TaskBase.PushChan)
}

func (task *Task) GoPush() {
	for i := 0; i < len(pushChannel); i++ {
		// 对每个管道初始化管道内消息数量，所以这个管道的Size应该就是限制能消费的个数
		pushChannel[i] = make(chan *PushParams, config.Conf.Task.TaskBase.PushChanSize)

		// 对每个管道进行啥操作这是？
		go task.processSinglePush(pushChannel[i])
	}
}

// 消费单聊消息？
func (task *Task) processSinglePush(ch chan *PushParams) {
	var arg *PushParams
	for {
		arg = <-ch
		//@todo when arg.ServerId server is down, user could be reconnect other serverId but msg in queue no consume
		// TODO：急需解决的问题：消息队列中的ID可能比服务器活得都久，如果服务器ID S1宕机，那么这些标注S1的消息，岂不是无处可去吗
		// 服务器宕机意味着Rooms中的所有房间都已经没了，为什么会没是因为Room中的用户全部下线了，既然都全下线了，那么单聊数据就无目的地了
		// 可以存入离线数据库，作为离线消息，在用户上线的时候加载离线消息即可
		// 还有一种情况：用户迁移服务器？但是不知道有没有这个功能？比较ServerID咋来的？似乎是开启的时候由作者自行添加的
		// 好像没有自动添加，或者说是自动扩容的功能
		// TODO：用户迁移，与服务器扩容
		// 这是将消息给推送到 ServerId服务器 上的 UserId用户 ？
		task.pushSingleToConnect(arg.ServerId, arg.UserId, arg.Msg)
	}
}

// 根据msg中的op类型推送消息到队列中，单聊消息推到channel中，然后消费者处理channel的消息
func (task *Task) Push(msg string) {
	m := &proto.RedisMsg{}
	if err := json.Unmarshal([]byte(msg), m); err != nil {
		logrus.Infof(" json.Unmarshal err:%v ", err)
	}
	logrus.Infof("push msg info %d,op is:%d", m.RoomId, m.Op)
	// 群聊消息都是直接发送，而单聊消息都是入管道？是的，方便对单聊做点操作 TODO：加密？
	// 我有个疑问，Connection都是在ServerID下；wait… 群聊消息并不是往一个房间丢消息，其他人去读取。而是往房间内的每个人发消息，
	// 所以是实时的，那似乎可能会因为延迟，导致某些人收到了消息，某些人没有收到
	// TODO：那么怎么解决这个问题，历史消息是要做的，公共历史消息吗，比如往公共筒子中放消息，然后由筒子去发给房间中的每个人，即使有延迟，短线重现的时候也可以通过筒子来复现历史消息
	switch m.Op {
	case config.OpSingleSend:
		pushChannel[rand.Int()%config.Conf.Task.TaskBase.PushChan] <- &PushParams{
			ServerId: m.ServerId,
			UserId:   m.UserId,
			Msg:      m.Msg,
		}
	case config.OpRoomSend:
		task.broadcastRoomToConnect(m.RoomId, m.Msg)
	case config.OpRoomCountSend:
		task.broadcastRoomCountToConnect(m.RoomId, m.Count)
	case config.OpRoomInfoSend:
		task.broadcastRoomInfoToConnect(m.RoomId, m.RoomUserInfo)
	}
}
