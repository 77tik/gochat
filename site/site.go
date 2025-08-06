/**
 * Created by lock
 * Date: 2019-08-12
 * Time: 11:36
 */
package site

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"io/ioutil"
	"net/http"
	"os"
	"path"
)

type Site struct {
}

func New() *Site {
	return &Site{}
}

// 自定义404处理逻辑
func notFound(w http.ResponseWriter, r *http.Request) {
	// Here you can send your custom 404 back.
	data, _ := ioutil.ReadFile("./site/index.html")
	_, _ = fmt.Fprintf(w, string(data)) // 控制台打印信息？
	return
}

// 挂载文件处理器
func server(fs http.FileSystem) http.Handler {
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 路径处理
		filePath := path.Clean("./site" + r.URL.Path)

		// 检查文件是否存在
		_, err := os.Stat(filePath)
		if err != nil {
			notFound(w, r) // 返回index.html?
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Site) Run() {
	siteConfig := config.Conf.Site
	port := siteConfig.SiteBase.ListenPort
	addr := fmt.Sprintf(":%d", port)
	logrus.Fatal(http.ListenAndServe(addr, server(http.Dir("./site"))))
}
