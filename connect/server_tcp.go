/**
 * Created by lock
 * Date: 2020/4/14
 */
package connect

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"gochat/api/rpc"
	"gochat/config"
	"gochat/pkg/stickpackage"
	"gochat/proto"
	"net"
	"strings"
	"time"
)

const maxInt = 1<<31 - 1

func init() {
	rpc.InitLogicRpcClient()
}

func (c *Connect) InitTcpServer() error {
	// 解析绑定多个地址
	aTcpAddr := strings.Split(config.Conf.Connect.ConnectTcp.Bind, ",")
	cpuNum := config.Conf.Connect.ConnectBucket.CpuNum
	var (
		addr     *net.TCPAddr
		listener *net.TCPListener
		err      error
	)
	for _, ipPort := range aTcpAddr {

		// 解析地址
		if addr, err = net.ResolveTCPAddr("tcp", ipPort); err != nil {
			logrus.Errorf("server_tcp ResolveTCPAddr error:%s", err.Error())
			return err
		}

		// 创建TCP监听
		if listener, err = net.ListenTCP("tcp", addr); err != nil {
			logrus.Errorf("net.ListenTCP(tcp, %s),error(%v)", ipPort, err)
			return err
		}
		logrus.Infof("start tcp listen at:%s", ipPort)
		// 按照指定的核心数启动监听协程
		for i := 0; i < cpuNum; i++ {
			go c.acceptTcp(listener)
		}
	}
	return nil
}

func (c *Connect) acceptTcp(listener *net.TCPListener) {
	var (
		conn *net.TCPConn
		err  error
		r    int
	)
	connectTcpConfig := config.Conf.Connect.ConnectTcp
	for {
		if conn, err = listener.AcceptTCP(); err != nil {
			logrus.Errorf("listener.Accept(\"%s\") error(%v)", listener.Addr().String(), err)
			return
		}
		// set keep alive，client==server ping package check
		// 启用TCP保活
		if err = conn.SetKeepAlive(connectTcpConfig.KeepAlive); err != nil {
			logrus.Errorf("conn.SetKeepAlive() error:%s", err.Error())
			return
		}
		//set ReceiveBuf
		if err := conn.SetReadBuffer(connectTcpConfig.ReceiveBuf); err != nil {
			logrus.Errorf("conn.SetReadBuffer() error:%s", err.Error())
			return
		}
		//set SendBuf
		if err := conn.SetWriteBuffer(connectTcpConfig.SendBuf); err != nil {
			logrus.Errorf("conn.SetWriteBuffer() error:%s", err.Error())
			return
		}

		// 启动处理协程
		go c.ServeTcp(DefaultServer, conn, r)
		if r++; r == maxInt {
			logrus.Infof("conn.acceptTcp num is:%d", r)
			r = 0
		}
	}
}

// 启动读写协程处理
func (c *Connect) ServeTcp(server *Server, conn *net.TCPConn, r int) {
	var ch *Channel
	ch = NewChannel(server.Options.BroadcastSize)
	ch.connTcp = conn
	// 进行消息推送，并维护心跳吗？ 每个套接字都存进一个Channel中了，然后为每个Channel处理单独的读写
	go c.writeDataToTcp(server, ch)

	// 处理消息解析或者触发连接管理吗？
	go c.readDataFromTcp(server, ch)
}

func (c *Connect) readDataFromTcp(s *Server, ch *Channel) {
	defer func() {
		// 连接断开时的清理逻辑
		logrus.Infof("start exec disConnect ...")
		if ch.Room == nil || ch.userId == 0 {
			logrus.Infof("roomId and userId eq 0")
			_ = ch.connTcp.Close()
			return
		}
		logrus.Infof("exec disConnect ...")
		disConnectRequest := new(proto.DisConnectRequest)
		disConnectRequest.RoomId = ch.Room.Id
		disConnectRequest.UserId = ch.userId

		// 筒子中删掉这个ch
		s.Bucket(ch.userId).DeleteChannel(ch)

		// rpc代理处理离开房间
		if err := s.operator.DisConnect(disConnectRequest); err != nil {
			logrus.Warnf("DisConnect rpc err :%s", err.Error())
		}
		if err := ch.connTcp.Close(); err != nil {
			logrus.Warnf("DisConnect close tcp conn err :%s", err.Error())
		}
		return
	}()
	// scanner
	// 创建粘包处理器，传入自定义分包逻辑
	scannerPackage := bufio.NewScanner(ch.connTcp)
	scannerPackage.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// 如果不是末尾，并且第一个字节不是v，v是干嘛的？
		// 我靠ai说这个是版本。。。
		// +--------+--------+----------------+
		//| 版本(2字节：v1)| 长度（记录的是总长）| 数据(N字节)    |
		//+--------+--------+----------------+
		if !atEOF && data[0] == 'v' {
			// 如果data长度大于4字节
			if len(data) > stickpackage.TcpHeaderLength {
				// 这是包总长吗？两字节的数字？
				packSumLength := int16(0)

				// 把字节数组(0x)开头的，读成数字，这里是大端读(符合人类的阅读顺序，123就是从左到右高位到低位)
				// 数据部分长度其实字节位置？从2开始读到下标4(不包含下标4)：{v 1} {0x00 0x02}=>读出来就是长度2
				_ = binary.Read(bytes.NewReader(data[stickpackage.LengthStartIndex:stickpackage.LengthStopIndex]), binary.BigEndian, &packSumLength)

				// 我靠按照这个的理论，它这个长度是还带了头的，不是只有data长度的，比如：前面2字节版本，2字节总长，N字节数据，但是这个总长代表的数字是2 + 2 + N=4 + N，所以这里就直接切了
				if int(packSumLength) <= len(data) {
					return int(packSumLength), data[:packSumLength], nil
				}
			}
		}
		return
	})
	scanTimes := 0
	for {
		scanTimes++
		if scanTimes > 3 {
			logrus.Infof("scannedPack times is:%d", scanTimes)
			break
		}
		// 根据分包逻辑，对每一个包进行针对性操作
		for scannerPackage.Scan() {
			scannedPack := new(stickpackage.StickPackage)
			// 调用一下Bytes就Split一下，然后返回出结果
			// 这里似乎只是检验一下是都读取不出错，貌似下下面才是读取数据
			// 并非！读到 scannedPack 这个结构体的字段内了！
			err := scannedPack.Unpack(bytes.NewReader(scannerPackage.Bytes()))
			if err != nil {
				logrus.Errorf("scan tcp package err:%s", err.Error())
				break
			}
			//get a full package
			var connReq proto.ConnectRequest
			logrus.Infof("get a tcp message :%s", scannedPack)
			var rawTcpMsg proto.SendTcp
			// 原来这个data部分也是一个结构体序列化来的，是TCP连接的元信息
			if err := json.Unmarshal([]byte(scannedPack.Msg), &rawTcpMsg); err != nil {
				logrus.Errorf("tcp message struct %+v", rawTcpMsg)
				break
			}
			logrus.Infof("json unmarshal,raw tcp msg is:%+v", rawTcpMsg)
			if rawTcpMsg.AuthToken == "" {
				logrus.Errorf("tcp s.operator.Connect no authToken")
				return
			}
			if rawTcpMsg.RoomId <= 0 {
				logrus.Errorf("tcp roomId not allow lgt 0")
				return
			}
			switch rawTcpMsg.Op {
			case config.OpBuildTcpConn:
				connReq.AuthToken = rawTcpMsg.AuthToken
				connReq.RoomId = rawTcpMsg.RoomId
				//fix
				//connReq.ServerId = config.Conf.Connect.ConnectTcp.ServerId
				connReq.ServerId = c.ServerId

				// 加入房间，其实就是rpc调用logic注册的服务
				userId, err := s.operator.Connect(&connReq)
				logrus.Infof("tcp s.operator.Connect userId is :%d", userId)
				if err != nil {
					logrus.Errorf("tcp s.operator.Connect error %s", err.Error())
					return
				}
				if userId == 0 {
					logrus.Error("tcp Invalid AuthToken ,userId empty")
					return
				}

				// 这是入桶吗？
				b := s.Bucket(userId)
				//insert into a bucket
				err = b.Put(userId, connReq.RoomId, ch)
				if err != nil {
					logrus.Errorf("tcp conn put room err: %s", err.Error())
					_ = ch.connTcp.Close()
					return
				}
			case config.OpRoomSend:
				//send tcp msg to room
				req := &proto.Send{
					Msg:          rawTcpMsg.Msg,
					FromUserId:   rawTcpMsg.FromUserId,
					FromUserName: rawTcpMsg.FromUserName,
					RoomId:       rawTcpMsg.RoomId,
					Op:           config.OpRoomSend,
				}

				// 这个rpc为什么是api层中的rpc实例？调用的还是logic在etcd中注册的服务
				code, msg := rpc.RpcLogicObj.PushRoom(req)
				logrus.Infof("tcp conn push msg to room,err code is:%d,err msg is:%s", code, msg)
			}
		}
		// 读到了一个空包EOF
		if err := scannerPackage.Err(); err != nil {
			logrus.Errorf("tcp get a err package:%s", err.Error())
			return
		}
	}
}

func (c *Connect) writeDataToTcp(s *Server, ch *Channel) {
	//ping time default 54s
	// 心跳间隔创建了一个计时器？
	ticker := time.NewTicker(DefaultServer.Options.PingPeriod)
	defer func() {
		// 计时器停止，然后关闭套接字
		ticker.Stop()
		_ = ch.connTcp.Close()
		return
	}()
	pack := stickpackage.StickPackage{
		Version: stickpackage.VersionContent,
	}
	for {
		select {
		// 从消息广播通道拿一个msg，这是发消息的
		case message, ok := <-ch.broadcast:
			if !ok {
				// 没消息了就直接关了，等等，怎么关了两次，这不是channel，所以重复关闭应该没事
				_ = ch.connTcp.Close()
				return
			}
			pack.Msg = message.Body
			pack.Length = pack.GetPackageLength()
			//send msg
			logrus.Infof("send tcp msg to conn:%s", pack.String())

			// pack的时候就直接通过套接字发走了
			// 打包和编解码终究不是一样的，打包就是单纯的一块一块发，编解码还会把这一块给编码成新的一块然后再发
			// 但终究发送的时候是要打包发的
			if err := pack.Pack(ch.connTcp); err != nil {
				logrus.Errorf("connTcp.write message err:%s", err.Error())
				return
			}
		case <-ticker.C: // 这是心跳保活，发ping msg，但是是发给谁的呢？
			// 也许是发给与服务器建立连接的客户端的，确认客户端是否存活吗？那么就还差 pong
			logrus.Infof("connTcp.ping message,send")
			//send a ping msg ,if error , return
			pack.Msg = []byte("ping msg")
			pack.Length = pack.GetPackageLength()
			if err := pack.Pack(ch.connTcp); err != nil {
				//send ping msg to tcp conn
				return
			}
		}
	}
}
