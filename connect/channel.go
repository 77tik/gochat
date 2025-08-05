/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 15:18
 */
package connect

import (
	"github.com/gorilla/websocket"
	"gochat/proto"
	"net"
)

// in fact, Channel it's a user Connect session
type Channel struct {
	Room      *Room // 有点意思，ROOM能用指针找到Channel，Channel还能找回去
	Next      *Channel
	Prev      *Channel
	broadcast chan *proto.Msg // 消息广播通道
	userId    int             // 用户ID
	conn      *websocket.Conn
	connTcp   *net.TCPConn
}

func NewChannel(size int) (c *Channel) {
	c = new(Channel)
	c.broadcast = make(chan *proto.Msg, size)
	c.Next = nil
	c.Prev = nil
	return
}

// 这里的链接究竟是谁的呢，如果是双方的，那为什么只有一个userid呢，如果不是单方的，那为什么这里说的是广播呢？
func (ch *Channel) Push(msg *proto.Msg) (err error) {
	select {
	case ch.broadcast <- msg:
	default:
	}
	return
}
