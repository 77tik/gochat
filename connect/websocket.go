/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 15:19
 */
package connect

import (
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"net/http"
)

func (c *Connect) InitWebsocket() error {
	// 注册ws路由
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c.serveWs(DefaultServer, w, r)
	})
	// 在配置地址上启动http服务
	err := http.ListenAndServe(config.Conf.Connect.ConnectWebsocket.Bind, nil)
	return err
}

func (c *Connect) serveWs(server *Server, w http.ResponseWriter, r *http.Request) {

	// 创建连接升级
	var upGrader = websocket.Upgrader{
		ReadBufferSize:  server.Options.ReadBufferSize,  // 读缓冲
		WriteBufferSize: server.Options.WriteBufferSize, // 写缓冲
	}
	//cross origin domain support
	// 允许跨域
	upGrader.CheckOrigin = func(r *http.Request) bool { return true }

	// 升级HTTP连接到WebSocket
	conn, err := upGrader.Upgrade(w, r, nil)

	if err != nil {
		logrus.Errorf("serverWs err:%s", err.Error())
		return
	}
	var ch *Channel
	//default broadcast size eq 512
	ch = NewChannel(server.Options.BroadcastSize)
	ch.conn = conn
	//send data to websocket conn
	go server.writePump(ch, c)
	//get data from websocket conn
	go server.readPump(ch, c)
}
