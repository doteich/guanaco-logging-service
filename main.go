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
	Subs   map[int]*monitor.Subscription
	File   string = *flag.String("file", "sample.json", "file name of config file")
	Logger *slog.Logger
)

func main() {

	flag.Parse()

	Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()

	var conf conf

	bArr, err := os.ReadFile(fmt.Sprintf("./configs/%s", File))

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
	}

	c, err := conf.CreateClient(ctx)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating client connection: %s", err.Error()))
		return
	}

	if err := c.Connect(ctx); err != nil {
		Logger.Error(fmt.Sprintf("error while creating client connection: %s", err.Error()))
		return
	}

	m, err := monitor.NewNodeMonitor(c)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating node monitor: %s", err.Error()))
		return
	}

	Subs = make(map[int]*monitor.Subscription)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go CreateSub(ctx, &wg, m, conf.MonitoredItems, conf.Interval, conf.DB)

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

func CreateSub(ctx context.Context, wg *sync.WaitGroup, m *monitor.NodeMonitor, nodes []string, iv int, db string) {
	sub, err := m.Subscribe(ctx, &opcua.SubscriptionParameters{Interval: time.Duration(iv) * time.Second}, func(s *monitor.Subscription, dcm *monitor.DataChangeMessage) {
		if dcm.Error != nil {
			Logger.Error(fmt.Sprintf("error with received sub message: %s - nodeid %s", dcm.Error.Error(), dcm.NodeID))
		} else {

			if dcm.Status != ua.StatusOK {
				Logger.Error(fmt.Sprintf("received bad status for sub message: %s - nodeid %s", dcm.Status, dcm.NodeID))
			} else {

			}
		}
	})

	if err != nil {
		Logger.Error(fmt.Sprintf("error while creating subscription: %s", err.Error()))
		defer TerminateSub(ctx, wg, sub)
	}

	for _, n := range nodes {
		_, err := sub.AddMonitorItems(ctx, monitor.Request{NodeID: ua.MustParseNodeID(n), MonitoringMode: ua.MonitoringModeReporting, MonitoringParameters: &ua.MonitoringParameters{DiscardOldest: true, QueueSize: 1}})

		if err != nil {
			Logger.Error(fmt.Sprintf("error adding subscription item: %s", err.Error()))
			continue
		}

	}

	Subs[len(Subs)+1] = sub

	defer TerminateSub(ctx, wg, sub)
	<-ctx.Done()
}

func TerminateSub(ctx context.Context, wg *sync.WaitGroup, s *monitor.Subscription) {
	wg.Done()
	s.Unsubscribe(ctx)
}
