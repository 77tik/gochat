/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 15:24
 */
package config

import (
	"github.com/spf13/viper"
	"os"
	"runtime"
	"strings"
	"sync"
)

var once sync.Once
var realPath string
var Conf *Config

const (
	SuccessReplyCode      = 0
	FailReplyCode         = 1
	SuccessReplyMsg       = "success"
	QueueName             = "gochat_queue"
	RedisBaseValidTime    = 86400
	RedisPrefix           = "gochat_"
	RedisRoomPrefix       = "gochat_room_"
	RedisRoomOnlinePrefix = "gochat_room_online_count_"
	MsgVersion            = 1
	OpSingleSend          = 2 // single user
	OpRoomSend            = 3 // send to room
	OpRoomCountSend       = 4 // get online user count
	OpRoomInfoSend        = 5 // send info to room
	OpBuildTcpConn        = 6 // build tcp conn
)

// 各个层的配置
type Config struct {
	Common  Common        // etcd and redis 配置 TODO：Mysql加进去
	Connect ConnectConfig // 连接层配置
	Logic   LogicConfig   // 逻辑层配置
	Task    TaskConfig    // Task处理与队列层配置
	Api     ApiConfig     // API层配置
	Site    SiteConfig    // Site层配置
}

func init() {
	Init()
}

// 通过栈帧获取当前目录名
func getCurrentDir() string {
	_, fileName, _, _ := runtime.Caller(1)
	aPath := strings.Split(fileName, "/")
	dir := strings.Join(aPath[0:len(aPath)-1], "/")
	return dir
}

func Init() {
	// 单例模式只做一次
	once.Do(func() {
		env := GetMode()
		//realPath, _ := filepath.Abs("./")
		// 获取当前文件目录，当前文件在config目录下，所以会获取到config目录
		realPath := getCurrentDir()
		// 如果env获取到的是dev(不设置的话默认是"dev")
		// 那么这里会得到：config/dev/
		configFilePath := realPath + "/" + env + "/"

		// 指定配置文件的格式为TOML
		viper.SetConfigType("toml")

		// 指定文件名(不含拓展名)
		viper.SetConfigName("/connect")

		// 添加配置文件搜索路径
		viper.AddConfigPath(configFilePath)

		// 读取并解析配置文件
		err := viper.ReadInConfig()
		if err != nil {
			panic(err)
		}

		// 将新配置文件合并到现有配置 合并common
		viper.SetConfigName("/common")
		err = viper.MergeInConfig()
		if err != nil {
			panic(err)
		}

		// 合并task
		viper.SetConfigName("/task")
		err = viper.MergeInConfig()
		if err != nil {
			panic(err)
		}

		// 合并logic
		viper.SetConfigName("/logic")
		err = viper.MergeInConfig()
		if err != nil {
			panic(err)
		}

		// 合并api
		viper.SetConfigName("/api")
		err = viper.MergeInConfig()
		if err != nil {
			panic(err)
		}

		// 合并site
		viper.SetConfigName("/site")
		err = viper.MergeInConfig()
		if err != nil {
			panic(err)
		}

		// 反序
		Conf = new(Config)
		viper.Unmarshal(&Conf.Common)
		viper.Unmarshal(&Conf.Connect)
		viper.Unmarshal(&Conf.Task)
		viper.Unmarshal(&Conf.Logic)
		viper.Unmarshal(&Conf.Api)
		viper.Unmarshal(&Conf.Site)
	})
}

// 获取环境参数 RUN_MODE
func GetMode() string {
	env := os.Getenv("RUN_MODE")
	if env == "" {
		env = "dev"
	}
	return env
}

// 获取GIN运行Mode
func GetGinRunMode() string {
	env := GetMode()
	//gin have debug,test,release mode
	if env == "dev" {
		return "debug"
	}
	if env == "test" {
		return "debug"
	}
	if env == "prod" {
		return "release"
	}
	return "release"
}

type CommonEtcd struct {
	Host              string `mapstructure:"host"`
	BasePath          string `mapstructure:"basePath"`
	ServerPathLogic   string `mapstructure:"serverPathLogic"`
	ServerPathConnect string `mapstructure:"serverPathConnect"`
	UserName          string `mapstructure:"userName"`
	Password          string `mapstructure:"password"`
	ConnectionTimeout int    `mapstructure:"connectionTimeout"`
}

type CommonRedis struct {
	RedisAddress  string `mapstructure:"redisAddress"`
	RedisPassword string `mapstructure:"redisPassword"`
	Db            int    `mapstructure:"db"`
}

type Common struct {
	CommonEtcd  CommonEtcd  `mapstructure:"common-etcd"`
	CommonRedis CommonRedis `mapstructure:"common-redis"`
}

type ConnectBase struct {
	CertPath string `mapstructure:"certPath"`
	KeyPath  string `mapstructure:"keyPath"`
}

type ConnectRpcAddressWebsockts struct {
	Address string `mapstructure:"address"`
}

type ConnectRpcAddressTcp struct {
	Address string `mapstructure:"address"`
}

type ConnectBucket struct {
	CpuNum        int    `mapstructure:"cpuNum"`
	Channel       int    `mapstructure:"channel"`
	Room          int    `mapstructure:"room"`
	SrvProto      int    `mapstructure:"svrProto"`
	RoutineAmount uint64 `mapstructure:"routineAmount"`
	RoutineSize   int    `mapstructure:"routineSize"`
}

type ConnectWebsocket struct {
	ServerId string `mapstructure:"serverId"`
	Bind     string `mapstructure:"bind"`
}

type ConnectTcp struct {
	ServerId      string `mapstructure:"serverId"`
	Bind          string `mapstructure:"bind"`
	SendBuf       int    `mapstructure:"sendbuf"`
	ReceiveBuf    int    `mapstructure:"receivebuf"`
	KeepAlive     bool   `mapstructure:"keepalive"`
	Reader        int    `mapstructure:"reader"`
	ReadBuf       int    `mapstructure:"readBuf"`
	ReadBufSize   int    `mapstructure:"readBufSize"`
	Writer        int    `mapstructure:"writer"`
	WriterBuf     int    `mapstructure:"writerBuf"`
	WriterBufSize int    `mapstructure:"writeBufSize"`
}

type ConnectConfig struct {
	ConnectBase                ConnectBase                `mapstructure:"connect-base"`
	ConnectRpcAddressWebSockts ConnectRpcAddressWebsockts `mapstructure:"connect-rpcAddress-websockts"`
	ConnectRpcAddressTcp       ConnectRpcAddressTcp       `mapstructure:"connect-rpcAddress-tcp"`
	ConnectBucket              ConnectBucket              `mapstructure:"connect-bucket"`
	ConnectWebsocket           ConnectWebsocket           `mapstructure:"connect-websocket"`
	ConnectTcp                 ConnectTcp                 `mapstructure:"connect-tcp"`
}

type LogicBase struct {
	ServerId   string `mapstructure:"serverId"`
	CpuNum     int    `mapstructure:"cpuNum"`
	RpcAddress string `mapstructure:"rpcAddress"`
	CertPath   string `mapstructure:"certPath"`
	KeyPath    string `mapstructure:"keyPath"`
}

type LogicConfig struct {
	LogicBase LogicBase `mapstructure:"logic-base"`
}

type TaskBase struct {
	CpuNum        int    `mapstructure:"cpuNum"`
	RedisAddr     string `mapstructure:"redisAddr"`
	RedisPassword string `mapstructure:"redisPassword"`
	RpcAddress    string `mapstructure:"rpcAddress"`
	PushChan      int    `mapstructure:"pushChan"`
	PushChanSize  int    `mapstructure:"pushChanSize"`
}

type TaskConfig struct {
	TaskBase TaskBase `mapstructure:"task-base"`
}

type ApiBase struct {
	ListenPort int `mapstructure:"listenPort"`
}

type ApiConfig struct {
	ApiBase ApiBase `mapstructure:"api-base"`
}

type SiteBase struct {
	ListenPort int `mapstructure:"listenPort"`
}

type SiteConfig struct {
	SiteBase SiteBase `mapstructure:"site-base"`
}
