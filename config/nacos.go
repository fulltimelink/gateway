package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-kratos/gateway/tools"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"

	configv1 "github.com/go-kratos/gateway/api/gateway/config/v1"
	knacos "github.com/go-kratos/kratos/contrib/config/nacos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"
)

type NacosLoader struct {
	source           config.Source
	confSHA256       string
	watchCancel      context.CancelFunc
	lock             sync.RWMutex
	onChangeHandlers []OnChange
}

func NewNacosLoader(confPath string) (*NacosLoader, error) {
	nacosIpAddr := tools.XlGetOsEnv("NACOS_IP_ADDR", "10.91.0.19")
	nacosPortStr := tools.XlGetOsEnv("NACOS_PORT", "30623")
	nacosNamespaceId := tools.XlGetOsEnv("NACOS_NAMESPACE_ID", "dx-transcode")
	nacosGroup := tools.XlGetOsEnv("NACOS_GROUP", "DEFAULT_GROUP")
	nacosDataId := tools.XlGetOsEnv("NACOS_DATA_ID", "config.yaml")
	nacosLogLevel := tools.XlGetOsEnv("NACOS_LOG_LEVEL", "debug")
	nacosPort, err := strconv.ParseUint(nacosPortStr, 10, 64)
	if nil != err {
		return nil, err
	}
	// --  @# nacos config source
	sc := []constant.ServerConfig{
		*constant.NewServerConfig(nacosIpAddr, nacosPort),
	}

	cc := &constant.ClientConfig{
		NamespaceId:         nacosNamespaceId,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "./nacos/log",
		CacheDir:            "./nacos/cache",
		LogLevel:            nacosLogLevel,
	}

	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		panic(err)
	}

	fl := &NacosLoader{
		source: knacos.NewConfigSource(
			client,
			knacos.WithGroup(nacosGroup),
			knacos.WithDataID(nacosDataId),
		),
	}
	if err := fl.initialize(); err != nil {
		return nil, err
	}
	return fl, nil
}

func (f *NacosLoader) initialize() error {
	sha256hex, err := f.configSHA256()
	if err != nil {
		return err
	}
	f.confSHA256 = sha256hex
	log.Infof("the initial config file sha256: %s", sha256hex)

	watchCtx, cancel := context.WithCancel(context.Background())
	f.watchCancel = cancel
	go f.watchproc(watchCtx)
	return nil
}

func (f *NacosLoader) configSHA256() (string, error) {
	configData, err := f.source.Load()
	if err != nil {
		return "", err
	}
	return sha256sum(configData[0].Value), nil
}

func (f *NacosLoader) Load(_ context.Context) (*configv1.Gateway, error) {
	log.Info("loading nacos config file")

	configData, err := f.source.Load()
	if err != nil {
		return nil, err
	}

	jsonData, err := yaml.YAMLToJSON(configData[0].Value)
	if err != nil {
		return nil, err
	}
	out := &configv1.Gateway{}
	if err := _jsonOptions.Unmarshal(jsonData, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *NacosLoader) Watch(fn OnChange) {
	log.Info("add config file change event handler")
	f.lock.Lock()
	defer f.lock.Unlock()
	f.onChangeHandlers = append(f.onChangeHandlers, fn)
}

func (f *NacosLoader) executeLoader() error {
	log.Info("execute config loader")
	f.lock.RLock()
	defer f.lock.RUnlock()

	var chainedError error
	for _, fn := range f.onChangeHandlers {
		if err := fn(); err != nil {
			log.Errorf("execute config loader error on handler: %+v: %+v", fn, err)
			chainedError = errors.New(err.Error())
		}
	}
	return chainedError
}

func (f *NacosLoader) watchproc(ctx context.Context) {
	log.Info("start watch config file")
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 5):
		}
		func() {
			sha256hex, err := f.configSHA256()
			if err != nil {
				log.Errorf("watch config file error: %+v", err)
				return
			}
			if sha256hex != f.confSHA256 {
				log.Infof("config file changed, reload config, last sha256: %s, new sha256: %s", f.confSHA256, sha256hex)
				if err := f.executeLoader(); err != nil {
					log.Errorf("execute config loader error with new sha256: %s: %+v, config digest will not be changed until all loaders are succeeded", sha256hex, err)
					return
				}
				f.confSHA256 = sha256hex
				return
			}
		}()
	}
}

func (f *NacosLoader) Close() {
	f.watchCancel()
}

type InspectNacosLoader struct {
	ConfPath         string `json:"confPath"`
	ConfSHA256       string `json:"confSha256"`
	OnChangeHandlers int64  `json:"onChangeHandlers"`
}

func (f *NacosLoader) DebugHandler() http.Handler {
	debugMux := http.NewServeMux()
	debugMux.HandleFunc("/debug/config/inspect", func(rw http.ResponseWriter, r *http.Request) {
		out := &InspectNacosLoader{
			ConfPath:         "f.confPath",
			ConfSHA256:       f.confSHA256,
			OnChangeHandlers: int64(len(f.onChangeHandlers)),
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(out)
	})
	debugMux.HandleFunc("/debug/config/load", func(rw http.ResponseWriter, r *http.Request) {
		out, err := f.Load(context.Background())
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte(err.Error()))
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		b, _ := protojson.Marshal(out)
		_, _ = rw.Write(b)
	})
	debugMux.HandleFunc("/debug/config/version", func(rw http.ResponseWriter, r *http.Request) {
		out, err := f.Load(context.Background())
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte(err.Error()))
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]interface{}{
			"version": out.Version,
		})
	})
	return debugMux
}
