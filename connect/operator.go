/**
 * Created by lock
 * Date: 2019-08-10
 * Time: 18:35
 */
package connect

import "gochat/proto"

// 操作符？这是什么形式，代理吗？
type Operator interface {
	Connect(conn *proto.ConnectRequest) (int, error)         // 用于加入房间请求
	DisConnect(disConn *proto.DisConnectRequest) (err error) // 用于离开房间请求
}

// 默认操作符只提供加入房间和离开房间的方法
type DefaultOperator struct {
}

// rpc call logic layer
func (o *DefaultOperator) Connect(conn *proto.ConnectRequest) (uid int, err error) {
	rpcConnect := new(RpcConnect)
	uid, err = rpcConnect.Connect(conn)
	return
}

// rpc call logic layer
func (o *DefaultOperator) DisConnect(disConn *proto.DisConnectRequest) (err error) {
	rpcConnect := new(RpcConnect)
	err = rpcConnect.DisConnect(disConn)
	return
}
