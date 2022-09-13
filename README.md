# Getting started

> 当前应用为一个`web` 服务器，通过监听`Rabbit MQ` 队列来获取需要生成`LWT` 海关查验信息文件，并通过将生成的`lwt`文件名发回到指定消息队列的形式通知生成结果。可通过`lwt`文件名，访问当前服务器提供的`API`接口下载对应的文件。





## 命令说明

通过命令启动服务器的同时，将异步起动`Rabbit MQ consumer` 监听程序，

- `cguard --config .cguard.yaml`

```shell
Usage:
  cguard [flags]

Flags:
      --config string   config file (default is .cguard.yaml) (default ".cguard.yaml")
  -h, --help            help for cguard

```

 

- 配置文件`.cguard.yaml`

```yaml
port: 7004

log:
  # logging level
  level: info
  # logging dir
  log-base: /var/log

# Import XML save directory
import-dir:

# Customs declaration file monitoring queue
rabbitmq:
  url: amqp://USER:PASSWORD@HOST:5672
  exchange: customs.lwt.direct
  exchange-type: direct
  queue:
    # lwt request queue name
    lwt-req: customs.lwt.request
    # lwt response queue name
    lwt-res: customs.lwt.response

mysql:
  driver: mysql
  url: 'USER:PASSWORD@tcp(HOST:PORT)/DATABASE'
  # connection max life time: default 3 minutes
  max-life-time: 3
  # max open connections: default 10
  max-open-connections: 10
  # max idle connections: default 10
  max-idle-connections: 10

# The path of lwt template
lwt:
  template:
    official:
      amazon: out/template/amazon.xlsx
      ebay: out/template/ebay.xlsx
      c_discount: out/template/cdiscount.xlsx
    brief:
      amazon: out/template/brief_amazon.xlsx
      ebay: out/template/brief_ebay.xlsx
      c_discount: out/template/brief_cdiscount.xlsx
  tmp:
    # LWT file save root directory
    dir: out/tmp


```



## 文件下载接口

- API

```http
GET http://localhost:7004/lwt/OP210603005_20220912104816.xlsx?download=1
Accept: application/json
```

- 文件名说明

$$
\overbrace{OP210603005}^{Customs ID}\_\overbrace{20220912104816}^{Create Time}.xlsx
$$

***注意：文件名结构均不可自行更改，以免查找不到文件***



## LWT 生成

`lwt` 制作请求，通过将需要制作`lwt` 的报关单单号发送到指定的消息队列来生成。

- `RequestLwt`

```go
type RequestForLwt struct {
	CustomsId string `json:"customs_id"`
	Brief     bool   `json:"brief"`
}
```

| 参数       | 值          | 说明             |
| ---------- | ----------- | ---------------- |
| customs_id | OP210603005 | 报关单号         |
| Brief      | Boolean     | 是否为Brief  LWT |

`lwt` 文件的生成结果通过监听队列`rqbbitmq.queue.lwt-res`变量指定的队列名获取。通过返回的数据内容，业务放自行保存`lwt` 业务状态及`lwt`文件名

- `ResponseLwt`

```go
// ResponseForLwt response for Lwt request
type ResponseForLwt struct {
	Status      string `json:"status"`
	LwtFilename string `json:"lwt_filename"`
	Error       string `json:"errors"`
}
```

| 参数         | 值                              | 说明                   |
| ------------ | ------------------------------- | ---------------------- |
| status       | Success \| failed               | L WT 是否成功          |
| lwt_filename | OP210603005_20220912104816.xlsx | LWT文件名              |
| error        | -                               | 如果失败，返回错误信息 |
| Brief        | Boolean                         | 是否为Brief  LWT       |

- json exp:

```json
{\"status\":\"success\",\"lwt_filename\":\"OP210603005_20220912130914.xlsx\",\"errors\":\"\"}
```

