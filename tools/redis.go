/**
 * Created by lock
 * Date: 2019-08-12
 * Time: 14:18
 */
package tools

import (
	"fmt"
	"github.com/go-redis/redis"
	"sync"
	"time"
)

// 这个map只在内部使用，那么它不是全局的？
var RedisClientMap = map[string]*redis.Client{}
var syncLock sync.Mutex

type RedisOption struct {
	Address  string
	Password string
	Db       int
}

// 初始化一个redis客户端，然后给他加到map里面，加map里面的操作是用锁的，所以是并发安全的
func GetRedisInstance(redisOpt RedisOption) *redis.Client {
	address := redisOpt.Address
	db := redisOpt.Db
	password := redisOpt.Password
	addr := fmt.Sprintf("%s", address)
	syncLock.Lock()
	if redisCli, ok := RedisClientMap[addr]; ok {
		return redisCli
	}
	client := redis.NewClient(&redis.Options{
		Addr:       addr,
		Password:   password,
		DB:         db,
		MaxConnAge: 20 * time.Second,
	})
	RedisClientMap[addr] = client
	syncLock.Unlock()
	return RedisClientMap[addr]
}
