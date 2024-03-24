package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"
)

type conf struct {
	ConfName       string   `json:"confName"`
	DB             string   `json:"db"`
	EP             string   `json:"ep"`
	Policy         string   `json:"policy"`
	Mode           string   `json:"mode"`
	Auth           string   `json:"auth"`
	Password       string   `json:"password"`
	Username       string   `json:"user"`
	MonitoredItems []string `json:"monitoredItems"`
	Interval       int      `json:"interval"`
}

var (
	Client        *opcua.Client
	Subs          map[uint32]*monitor.Subscription
	File          = flag.String("file", "./configs/sample.json", "file name of config file")
	Logger        *slog.Logger
	LastKeepAlive time.Time
	RetryActive   bool
)

func main() {

	flag.Parse()

	Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx := context.Background()

	var conf conf

	bArr, err := os.ReadFile(*File)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while reading config file: %s", err.Error()))
		return
	}

	if err := json.Unmarshal(bArr, &conf); err != nil {
		Logger.Error(fmt.Sprintf("config contains invalid json: %s", err.Error()))
		return
	}

	if err := InitDB(ctx, conf.ConfName, conf.DB); err != nil {
		Logger.Error(fmt.Sprintf("error creating db: %s", err.Error()))
		return
	}

	Client, err = conf.CreateClient(ctx)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating client connection: %s", err.Error()))
		return
	}

	if err := Client.Connect(ctx); err != nil {
		Logger.Error(fmt.Sprintf("error while creating client connection: %s", err.Error()))
		return
	}

	Subs = make(map[uint32]*monitor.Subscription)

	InitSubs(ctx, conf)

	//go queryAll(ctx)

	wg := sync.WaitGroup{}
	wg.Add(1)

	tick := time.NewTicker(30 * time.Second)

	go ConnectionCheck(tick, ctx, &wg, conf)

	wg.Wait()
}

func (p *conf) CreateClient(ctx context.Context) (*opcua.Client, error) {
	eps, err := opcua.GetEndpoints(ctx, p.EP)

	if err != nil {
		return nil, err
	}

	if len(eps) < 1 {
		return nil, errors.New("no endpoints found")
	}

	ep := opcua.SelectEndpoint(eps, p.Policy, ua.MessageSecurityModeFromString(p.Mode))

	opts := []opcua.Option{
		opcua.ApplicationName("guanaco"),
		opcua.AutoReconnect(true),
		opcua.ReconnectInterval(10 * time.Second),
		opcua.SecurityPolicy(p.Policy),
		opcua.SecurityMode(ua.MessageSecurityModeFromString(p.Mode)),
	}

	switch p.Auth {
	case "Anonymous":
		opts = append(opts, opcua.AuthAnonymous())
		opts = append(opts, opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous))
	case "User&Password":
		opts = append(opts, opcua.AuthUsername(p.Username, p.Password))
		opts = append(opts, opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName))
	}

	if p.Policy != "None" {
		opts = append(opts, opcua.CertificateFile("./certs/cert.pem"))
		opts = append(opts, opcua.PrivateKeyFile("./certs/key.pem"))
	}

	c, err := opcua.NewClient(p.EP, opts...)

	if err != nil {
		return nil, err
	}

	return c, err
}

func InitSubs(ctx context.Context, conf conf) {
	m, err := monitor.NewNodeMonitor(Client)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating node monitor: %s", err.Error()))
		return
	}

	go CreateSub(ctx, m, conf.MonitoredItems, conf.Interval, conf.DB)
	go KeepAlive(ctx, m)
}

func CreateSub(ctx context.Context, m *monitor.NodeMonitor, nodes []string, iv int, db string) {
	sub, err := m.Subscribe(ctx, &opcua.SubscriptionParameters{Interval: time.Duration(iv) * time.Second}, func(s *monitor.Subscription, dcm *monitor.DataChangeMessage) {
		if dcm.Error != nil {
			Logger.Error(fmt.Sprintf("error with received sub message: %s - nodeid %s", dcm.Error.Error(), dcm.NodeID))
		} else if dcm.Status != ua.StatusOK {
			Logger.Error(fmt.Sprintf("received bad status for sub message: %s - nodeid %s", dcm.Status, dcm.NodeID))
		} else {

			var dt string

			switch dcm.Value.Value().(type) {
			case int:
				dt = "Int"
			case uint8:
				dt = "u8"
			case uint16:
				dt = "u16"
			case uint32:
				dt = "u32"
			case int8:
				dt = "i8"
			case int16:
				dt = "i16"
			case int32:
				dt = "i32"
			case int64:
				dt = "i64"
			case float32:
				dt = "f32"
			case float64:
				dt = "f64"
			case bool:
				dt = "Bool"
			default:
				dt = "Str"

			}
			p := payload{ts: dcm.SourceTimestamp, nodeId: dcm.NodeID.String(), dataType: dt, value: dcm.Value.Value(), nodeName: dcm.NodeID.StringID()}
			p.InsertData(ctx)

		}

	})

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating subscription: %s", err.Error()))

		return
	}

	for _, n := range nodes {
		_, err := sub.AddMonitorItems(ctx, monitor.Request{NodeID: ua.MustParseNodeID(n), MonitoringMode: ua.MonitoringModeReporting, MonitoringParameters: &ua.MonitoringParameters{DiscardOldest: true, QueueSize: 1}})

		if err != nil {
			Logger.Error(fmt.Sprintf("error adding subscription item: %s", err.Error()))
			continue
		}

	}

	id := sub.SubscriptionID()
	Subs[id] = sub

	defer TerminateSub(ctx, sub, id)
	<-ctx.Done()
}

func TerminateSub(ctx context.Context, s *monitor.Subscription, id uint32) {
	delete(Subs, id)
	s.Unsubscribe(ctx)

}

func KeepAlive(ctx context.Context, m *monitor.NodeMonitor) {

	LastKeepAlive = time.Now()

	sub, err := m.Subscribe(ctx, &opcua.SubscriptionParameters{Interval: 10 * time.Second}, func(s *monitor.Subscription, dcm *monitor.DataChangeMessage) {
		if dcm.Error != nil {
			Logger.Error(fmt.Sprintf("error with received keepalive message: %s - nodeid %s", dcm.Error.Error(), dcm.NodeID))
		} else if dcm.Status != ua.StatusOK {
			Logger.Error(fmt.Sprintf("received bad status for keepalive message: %s - nodeid %s", dcm.Value.StatusCode(), dcm.NodeID))
		} else {
			LastKeepAlive = time.Now()
		}
	})

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating subscription: %s", err.Error()))
		return
	}

	sub.AddMonitorItems(ctx, monitor.Request{NodeID: ua.MustParseNodeID("i=2258"), MonitoringMode: ua.MonitoringModeReporting, MonitoringParameters: &ua.MonitoringParameters{DiscardOldest: true, QueueSize: 1}})

	id := sub.SubscriptionID()
	Subs[id] = sub

	defer TerminateSub(ctx, sub, id)
	<-ctx.Done()

}

func ConnectionCheck(t *time.Ticker, ctx context.Context, wg *sync.WaitGroup, conf conf) {
	for {
		select {
		case <-t.C:
			diff := time.Since(LastKeepAlive).Seconds()

			if diff > 60 {
				Logger.Warn("last keepalive message received over 60s ago - reinit subs")
				for id, sub := range Subs {
					TerminateSub(ctx, sub, id)
				}
				InitSubs(ctx, conf)

			}

		case <-ctx.Done():
			wg.Done()
			return
		}
	}
}
