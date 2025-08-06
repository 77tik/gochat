## 一些奇怪的问题

+ 传给前端json响应的时候，有很多字段用的是 *float 类型，我查了一下是因为float如果不传就默认是 0
  + 但是有些参数你float是0，就不太好，所以把它改成 *float , 这样你不传就会变成nil, 前端就知道这是无数据的
  + 数据库里面字段类型则填 DOUBLE NULL 
+ go-zero的fileserver有两种形式
  + 一种是把文件下载注册成一个路由，然后在中间件中启动gzip，这样就可以通过gzip传输文件
  + 另一种是使用WithFileServer，这个则需要使用Option注入配置，此时中间件的gzip就不起作用了，那么需要手动写一个配置
  + 然后在配置中将Server内部的server变成我们的gzipServer，这里我选择使用unsafePointer构造一个结构一样的Server，然后使用
  + 指针强转，替换原本的Server为新类型中的gzipServer
  + 挂载路径以及文件关联，然后在配置的中间件Serve中的Middleware中，根据路径判断是否符合，然后传输，传输完以后就会return，不会再进入下一个中间件中
+ golang文件传输与挂载：
  + 再潦河上我写了一个下载文件的接口，那里似乎用到了类似于embed.Fs 的构造，原理是把某些目录打包进二进制文件，让golang可以直接读取
  + 相当于一个虚拟的文件系统
+ golang的并发：
  + 在泸定中设计了一个errgroup的操作用于并发提交作业，并且遇到错误就会回退？
+ 责任链：
  + 潦河责任链设计