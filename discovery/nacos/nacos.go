package nacos

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/go-kratos/gateway/discovery"
	"github.com/go-kratos/kratos/contrib/registry/nacos/v2"
	"github.com/go-kratos/kratos/v2/registry"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
)

func init() {
	discovery.Register("nacos", New)
}

func New(dsn *url.URL) (registry.Discovery, error) {
	theHost := strings.Split(dsn.Host, ":")
	ipAddr := theHost[0]
	logDir := dsn.Query().Get("logdir")
	cacheDir := dsn.Query().Get("cachedir")
	logLevel := dsn.Query().Get("loglevel")
	port, err := strconv.ParseUint(theHost[1], 10, 64)
	timeout, err := strconv.ParseUint(dsn.Query().Get("port"), 10, 64)
	notLoadCacheAtStart, err := strconv.ParseBool(dsn.Query().Get("notloadcacheatstart"))
	if nil != err {
		return nil, err
	}

	namespaceId := dsn.Query().Get("namespaceid")
	sc := []constant.ServerConfig{
		*constant.NewServerConfig(ipAddr, port),
	}
	cc := constant.ClientConfig{
		NamespaceId:         namespaceId,
		TimeoutMs:           timeout,
		NotLoadCacheAtStart: notLoadCacheAtStart,
		LogDir:              logDir,
		CacheDir:            cacheDir,
		LogLevel:            logLevel,
	}
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return nil, err
	}
	r := nacos.New(client)
	return r, nil
}
