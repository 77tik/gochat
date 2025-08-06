/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 18:22
 */
package task

import (
	"github.com/sirupsen/logrus"
	"gochat/config"
	"runtime"
)

type Task struct {
}

func New() *Task {
	return new(Task)
}

func (task *Task) Run() {
	//read config
	taskConfig := config.Conf.Task
	runtime.GOMAXPROCS(taskConfig.TaskBase.CpuNum)
	//read from redis queue
	// 开启消费者，群聊消息由消费者直接发送，而单聊消息则被消费者丢入管道，由下面的GoPush去读取管道然后发送
	if err := task.InitQueueRedisClient(); err != nil {
		logrus.Panicf("task init publishRedisClient fail,err:%s", err.Error())
	}
	//rpc call connect layer send msg
	// 向connect发送消息，我记得connection层似乎有一个server来着？
	// 有两个server，一个是tcp一个是ws，似乎就是留给task调用的
	if err := task.InitConnectRpcClient(); err != nil {
		logrus.Panicf("task init InitConnectRpcClient fail,err:%s", err.Error())
	}
	//GoPush 专门为了单聊消息的发送制作的管道
	task.GoPush()
}
