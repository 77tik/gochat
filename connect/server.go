/**
 * Created by lock
 * Date: 2019-08-10
 * Time: 18:32
 */
package connect

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"gochat/proto"
	"gochat/tools"
	"time"
)

// TODO：可以用函数注入方式写的更装一些
type Server struct {
	Buckets []*Bucket // 使用多个筒子分散存储连接，减少锁竞争？具体用法我来看看怎么实现的
	// 我看懂了，每个Bucket会管理一些ROOM，ROOM则以链表形式管理Channel
	// 而且Buket也会直接记录userID => Channel的关联，方便删除，和找到其他Channel
	Options   ServerOptions // 服务器配置
	bucketIdx uint32        // 筒子数量
	operator  Operator      // RPC操作接口？ 具体是啥还得看一下
	// 我看完了，这是用来调用logic层在etcd注册的方法的，目前我们在Connection层只能调用加入房间和离开房间两个方法
	// 所以它是一个RPC操作符
}

type ServerOptions struct {
	WriteWait       time.Duration // 写超时
	PongWait        time.Duration // Pong响应超时？我记得Pong是在ping之后要的返回类型，那么这是否是用于心跳呢
	PingPeriod      time.Duration // 心跳间隔，这是留给tcp连接中服务器是不是会ping一下那一头
	MaxMessageSize  int64         // 最大消息大小
	ReadBufferSize  int           // 读缓冲
	WriteBufferSize int           // 写缓冲
	BroadcastSize   int           // 广播队列大小？？
}

func NewServer(b []*Bucket, o Operator, options ServerOptions) *Server {
	s := new(Server)
	s.Buckets = b
	s.Options = options
	s.bucketIdx = uint32(len(b))
	s.operator = o
	return s
}

// reduce lock competition, use google city hash insert to different bucket
// 用奇怪的hash函数？算出hash值作为筒子索引，然后把这个筒子返回出去，似乎是要做一些操作
func (s *Server) Bucket(userId int) *Bucket {
	userIdStr := fmt.Sprintf("%d", userId)
	idx := tools.CityHash32([]byte(userIdStr), uint32(len(userIdStr))) % s.bucketIdx
	return s.Buckets[idx]
}

// tcp同款写通道，不过似乎有一些变化
func (s *Server) writePump(ch *Channel, c *Connect) {
	//PingPeriod default eq 54s
	ticker := time.NewTicker(s.Options.PingPeriod)
	defer func() {
		ticker.Stop()
		ch.conn.Close()
	}()
	// 1.变化：没有打包发送？
	for {
		select {
		case message, ok := <-ch.broadcast:
			//write data dead time , like http timeout , default 10s
			ch.conn.SetWriteDeadline(time.Now().Add(s.Options.WriteWait))
			if !ok {
				logrus.Warn("SetWriteDeadline not ok")
				ch.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 消息分帧？
			w, err := ch.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				logrus.Warn(" ch.conn.NextWriter err :%s  ", err.Error())
				return
			}
			logrus.Infof("message write body:%s", message.Body)
			w.Write(message.Body)
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			//heartbeat，if ping error will exit and close current websocket conn
			ch.conn.SetWriteDeadline(time.Now().Add(s.Options.WriteWait))
			logrus.Infof("websocket.PingMessage :%v", websocket.PingMessage)
			if err := ch.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) readPump(ch *Channel, c *Connect) {
	defer func() {
		logrus.Infof("start exec disConnect ...")
		if ch.Room == nil || ch.userId == 0 {
			logrus.Infof("roomId and userId eq 0")
			ch.conn.Close()
			return
		}
		logrus.Infof("exec disConnect ...")
		disConnectRequest := new(proto.DisConnectRequest)
		disConnectRequest.RoomId = ch.Room.Id
		disConnectRequest.UserId = ch.userId
		s.Bucket(ch.userId).DeleteChannel(ch)
		if err := s.operator.DisConnect(disConnectRequest); err != nil {
			logrus.Warnf("DisConnect err :%s", err.Error())
		}
		ch.conn.Close()
	}()

	ch.conn.SetReadLimit(s.Options.MaxMessageSize)
	// 设置读超时
	ch.conn.SetReadDeadline(time.Now().Add(s.Options.PongWait))

	// 这是在干嘛？
	ch.conn.SetPongHandler(func(string) error {
		ch.conn.SetReadDeadline(time.Now().Add(s.Options.PongWait))
		return nil
	})

	for {
		_, message, err := ch.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logrus.Errorf("readPump ReadMessage err:%s", err.Error())
				return
			}
		}
		if message == nil {
			return
		}
		var connReq *proto.ConnectRequest
		logrus.Infof("get a message :%s", message)
		if err := json.Unmarshal([]byte(message), &connReq); err != nil {
			logrus.Errorf("message struct %+v", connReq)
		}
		if connReq == nil || connReq.AuthToken == "" {
			logrus.Errorf("s.operator.Connect no authToken")
			return
		}
		connReq.ServerId = c.ServerId //config.Conf.Connect.ConnectWebsocket.ServerId
		userId, err := s.operator.Connect(connReq)
		if err != nil {
			logrus.Errorf("s.operator.Connect error %s", err.Error())
			return
		}
		if userId == 0 {
			logrus.Error("Invalid AuthToken ,userId empty")
			return
		}
		logrus.Infof("websocket rpc call return userId:%d,RoomId:%d", userId, connReq.RoomId)
		b := s.Bucket(userId)
		//insert into a bucket
		err = b.Put(userId, connReq.RoomId, ch)
		if err != nil {
			logrus.Errorf("conn close err: %s", err.Error())
			ch.conn.Close()
		}
	}
}
