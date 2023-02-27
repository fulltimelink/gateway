package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/fulltimelink/gateway/client"
	"github.com/fulltimelink/gateway/config"
	configLoader "github.com/fulltimelink/gateway/config/config-loader"
	"github.com/fulltimelink/gateway/discovery"
	"github.com/fulltimelink/gateway/middleware"
	"github.com/fulltimelink/gateway/proxy"
	"github.com/fulltimelink/gateway/proxy/debug"
	"github.com/fulltimelink/gateway/server"

	_ "net/http/pprof"

	_ "github.com/fulltimelink/gateway/discovery/consul"
	_ "github.com/fulltimelink/gateway/discovery/nacos"
	_ "github.com/fulltimelink/gateway/middleware/bbr"
	"github.com/fulltimelink/gateway/middleware/circuitbreaker"
	_ "github.com/fulltimelink/gateway/middleware/cors"
	_ "github.com/fulltimelink/gateway/middleware/dxauth"
	_ "github.com/fulltimelink/gateway/middleware/logging"
	_ "github.com/fulltimelink/gateway/middleware/rewrite"
	_ "github.com/fulltimelink/gateway/middleware/tracing"
	_ "github.com/fulltimelink/gateway/middleware/transcoder"
	_ "go.uber.org/automaxprocs"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/registry"
	"github.com/go-kratos/kratos/v2/transport"
)

var (
	ctrlName     string
	ctrlService  string
	discoveryDSN string
	proxyAddrs   = newSliceVar(":8080")
	proxyConfig  string
	withDebug    bool
)

type sliceVar struct {
	val        []string
	defaultVal []string
}

func newSliceVar(defaultVal ...string) sliceVar {
	return sliceVar{defaultVal: defaultVal}
}
func (s *sliceVar) Get() []string {
	if len(s.val) <= 0 {
		return s.defaultVal
	}
	return s.val
}
func (s *sliceVar) Set(val string) error {
	s.val = append(s.val, val)
	return nil
}
func (s *sliceVar) String() string { return fmt.Sprintf("%+v", *s) }

func init() {
	flag.BoolVar(&withDebug, "debug", false, "enable debug handlers")
	flag.Var(&proxyAddrs, "addr", "proxy address, eg: -addr 0.0.0.0:8080")
	flag.StringVar(&proxyConfig, "conf", "config.yaml", "config path, eg: -conf config.yaml")
	flag.StringVar(&ctrlName, "ctrl.name", os.Getenv("ADVERTISE_NAME"), "control gateway name, eg: gateway")
	flag.StringVar(&ctrlService, "ctrl.service", "", "control service host, eg: http://127.0.0.1:8000")
	flag.StringVar(&discoveryDSN, "discovery.dsn", "", "discovery dsn, eg: consul://127.0.0.1:7070?token=secret&datacenter=prod")
}

func makeDiscovery() registry.Discovery {
	if discoveryDSN == "" {
		return nil
	}
	d, err := discovery.Create(discoveryDSN)
	if err != nil {
		log.Fatalf("failed to create discovery: %v", err)
	}
	return d
}

func main() {
	flag.Parse()

	clientFactory := client.NewFactory(makeDiscovery())
	p, err := proxy.New(clientFactory, middleware.Create)
	if err != nil {
		log.Fatalf("failed to new proxy: %v", err)
	}
	circuitbreaker.Init(clientFactory)

	ctx := context.Background()
	var ctrlLoader *configLoader.CtrlConfigLoader
	if ctrlService != "" {
		log.Infof("setup control service to: %q", ctrlService)
		ctrlLoader = configLoader.New(ctrlName, ctrlService, proxyConfig)
		if err := ctrlLoader.Load(ctx); err != nil {
			log.Errorf("failed to do initial load from control service: %v, using local config instead", err)
		}
		go ctrlLoader.Run(ctx)
	}

	//confLoader, err := config.NewFileLoader(proxyConfig)
	confLoader, err := config.NewNacosLoader(proxyConfig)
	if err != nil {
		log.Fatalf("failed to create config file loader: %v", err)
	}
	defer confLoader.Close()
	bc, err := confLoader.Load(context.Background())
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := p.Update(bc); err != nil {
		log.Fatalf("failed to update service config: %v", err)
	}
	reloader := func() error {
		bc, err := confLoader.Load(context.Background())
		if err != nil {
			log.Errorf("failed to load config: %v", err)
			return err
		}
		if err := p.Update(bc); err != nil {
			log.Errorf("failed to update service config: %v", err)
			return err
		}
		log.Infof("config reloaded")
		return nil
	}
	confLoader.Watch(reloader)

	var serverHandler http.Handler = p
	if withDebug {
		debug.Register("proxy", p)
		debug.Register("config", confLoader)
		if ctrlLoader != nil {
			debug.Register("ctrl", ctrlLoader)
		}
		serverHandler = debug.MashupWithDebugHandler(p)
	}
	servers := make([]transport.Server, 0, len(proxyAddrs.Get()))
	for _, addr := range proxyAddrs.Get() {
		servers = append(servers, server.NewProxy(serverHandler, addr))
	}
	// --  @# 追加metrics server
	servers = append(servers, server.NewMetrics())
	app := kratos.New(
		kratos.Name(bc.Name),
		kratos.Context(ctx),
		kratos.Server(
			servers...,
		),
	)
	if err := app.Run(); err != nil {
		log.Errorf("failed to run servers: %v", err)
	}
}
