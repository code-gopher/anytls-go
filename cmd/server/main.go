package main

import (
	"anytls/proxy/padding"
	"anytls/util"
	"anytls/v2board"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

func main() {
	// ---- 通用参数 ----
	listen := flag.String("l", "0.0.0.0:8443", "server listen port")
	password := flag.String("p", "", "password (used in plain mode)")
	paddingScheme := flag.String("padding-scheme", "", "padding-scheme file path")

	// ---- V2board 参数 ----
	v2boardApiHost := flag.String("v2board-api-host", "", "V2board 面板地址，如 https://panel.example.com")
	v2boardApiKey := flag.String("v2board-api-key", "", "V2board API 密钥")
	v2boardNodeID := flag.Uint("v2board-node-id", 0, "V2board 节点 ID")
	v2boardPullInterval := flag.Duration("v2board-pull-interval", 60*time.Second, "用户列表拉取周期（如 60s）")
	v2boardPushInterval := flag.Duration("v2board-push-interval", 60*time.Second, "流量上报周期（如 60s）")

	flag.Parse()

	// ---- 日志级别 ----
	logLevel, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logrus.SetLevel(logLevel)

	// ---- 填充方案（可选） ----
	if *paddingScheme != "" {
		f, err := os.Open(*paddingScheme)
		if err != nil {
			logrus.Fatalln("打开 padding-scheme 文件失败:", err)
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			logrus.Fatalln("读取 padding-scheme 文件失败:", err)
		}
		if padding.UpdatePaddingScheme(b) {
			logrus.Infoln("已加载自定义填充方案:", *paddingScheme)
		} else {
			logrus.Errorln("填充方案格式错误:", *paddingScheme)
		}
	}

	// ---- 使用 V2board 模式 ----
	isV2boardMode := *v2boardApiHost != "" && *v2boardApiKey != "" && *v2boardNodeID != 0

	// 判断参数完整性
	if isV2boardMode {
		logrus.Infof("[Server] %s (V2board 模式)", util.ProgramVersionName)
		if *v2boardNodeID == 0 {
			logrus.Fatalln("V2board 模式下必须指定 --v2board-node-id")
		}
	} else {
		// 普通密码模式必须提供密码
		if *password == "" {
			logrus.Fatalln("请通过 -p 指定密码，或使用 --v2board-* 参数启用 V2board 模式")
		}
		logrus.Infof("[Server] %s (普通密码模式)", util.ProgramVersionName)
	}

	// ---- 从 V2board 拉取节点配置（覆盖监听端口等） ----
	if isV2boardMode {
		apiClient := v2board.NewClient(*v2boardApiHost, *v2boardApiKey, *v2boardNodeID)
		nodeInfo, err := apiClient.GetNodeInfo()
		if err != nil {
			logrus.Warnf("[V2board] 拉取节点配置失败（使用默认参数继续）: %v", err)
		} else {
			logrus.Infof("[V2board] 节点配置已拉取，节点端口: %d", nodeInfo.ServerPort)
			if nodeInfo.ServerPort > 0 {
				*listen = ":" + formatUint(nodeInfo.ServerPort)
			}
			// 若面板配置了同步周期，则使用面板值（命令行参数优先）
			if *v2boardPullInterval == 60*time.Second && nodeInfo.BaseConfig.PullInterval > 0 {
				*v2boardPullInterval = time.Duration(nodeInfo.BaseConfig.PullInterval) * time.Second
			}
			if *v2boardPushInterval == 60*time.Second && nodeInfo.BaseConfig.PushInterval > 0 {
				*v2boardPushInterval = time.Duration(nodeInfo.BaseConfig.PushInterval) * time.Second
			}
		}
	}

	logrus.Infoln("[Server] 监听 TCP", *listen)

	// ---- 创建 TCP 监听 ----
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		logrus.Fatalln("监听 TCP 失败:", err)
	}

	// ---- 生成自签名 TLS 证书 ----
	tlsCert, _ := util.GenerateKeyPair(time.Now, "")
	tlsConfig := &tls.Config{
		GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return tlsCert, nil
		},
	}

	ctx := context.Background()
	var server *myServer

	if isV2boardMode {
		apiClient := v2board.NewClient(*v2boardApiHost, *v2boardApiKey, *v2boardNodeID)
		authMgr := v2board.NewAuthManager(apiClient)
		trafficMgr := v2board.NewTrafficManager(apiClient)

		// 启动定时拉取用户列表（阻塞直到首次拉取成功可在 Start 内处理）
		go authMgr.Start(*v2boardPullInterval)
		// 启动定时流量上报
		go trafficMgr.Start(*v2boardPushInterval)

		server = NewMyServerV2board(tlsConfig, authMgr, trafficMgr)
	} else {
		sum := sha256.Sum256([]byte(*password))
		server = NewMyServer(tlsConfig, sum[:])
	}

	// ---- 主循环：接受连接 ----
	for {
		c, err := listener.Accept()
		if err != nil {
			logrus.Fatalln("accept:", err)
		}
		go handleTcpConnection(ctx, c, server)
	}
}

// formatUint 将 uint 转为字符串（避免引入 strconv 额外依赖）
func formatUint(n uint) string {
	return fmt.Sprintf("%d", n)
}
