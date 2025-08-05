/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 15:18
 */
package connect

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gochat/proto"
	"sync"
)

const NoRoom = -1

type Room struct {
	Id          int // 房间ID
	OnlineCount int // 房间在线人数，但是怎么判断是否在线？
	rLock       sync.RWMutex
	drop        bool     // 房间是否存活的标记
	next        *Channel // 首个Channel指针，原来这东西是dummy头节点
}

func NewRoom(roomId int) *Room {
	room := new(Room)
	room.Id = roomId
	room.drop = false
	room.next = nil
	room.OnlineCount = 0
	return room
}

// 加入链表管理，无尾部节点，只能从头部开始找
func (r *Room) Put(ch *Channel) (err error) {
	//doubly linked list
	r.rLock.Lock()
	defer r.rLock.Unlock()
	if !r.drop {
		if r.next != nil {
			r.next.Prev = ch
		}
		ch.Next = r.next
		ch.Prev = nil
		r.next = ch
		r.OnlineCount++
	} else {
		err = errors.New("room drop")
	}
	return
}

// 消息推送，Connect层已经是离客户端最近的了，所以这里就直接传输过去了，从头节点往后找一个一个push
// TODO：也许我们可以优化一下让他推的更快，这里是O(N)
func (r *Room) Push(msg *proto.Msg) {
	r.rLock.RLock()
	for ch := r.next; ch != nil; ch = ch.Next {
		if err := ch.Push(msg); err != nil {
			logrus.Infof("push msg err:%s", err.Error())
		}
	}
	r.rLock.RUnlock()
	return
}

// 删除链表上的一个节点
func (r *Room) DeleteChannel(ch *Channel) bool {
	r.rLock.RLock()
	if ch.Next != nil {
		//if not footer
		ch.Next.Prev = ch.Prev
	}
	if ch.Prev != nil {
		// if not header
		ch.Prev.Next = ch.Next
	} else {
		r.next = ch.Next
	}
	r.OnlineCount--
	r.drop = false
	if r.OnlineCount <= 0 {
		r.drop = true
	}
	r.rLock.RUnlock()
	return r.drop
}
