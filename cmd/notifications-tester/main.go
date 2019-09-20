package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

type previewHandler struct {
	logger log.Logger
}

func (h previewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO remove
	w.Header().Set("Access-Control-Allow-Origin", "*")

	amCfg := r.FormValue("amconfig")
	cfg, err := config.Load(amCfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading Alertmanager configuration: %v", err), http.StatusBadRequest)
		return
	}

	alerts := r.FormValue("alerts")
	var as []*types.Alert
	if err := json.Unmarshal([]byte(alerts), &as); err != nil {
		http.Error(w, fmt.Sprintf("error parsing alerts: %v", err), http.StatusBadRequest)
		return
	}
	if len(as) == 0 {
		http.Error(w, "no alerts defined", http.StatusBadRequest)
		return
	}

	groupBy := strings.Split(r.FormValue("group-by"), ",")
	groupLabels := model.LabelSet{}
	for _, l := range groupBy {
		ln := model.LabelName(strings.TrimSpace(l))
		groupLabels[ln] = as[0].Labels[ln]
	}

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "test-groupkey")
	ctx = notify.WithGroupLabels(ctx, groupLabels)

	var slackCfg *config.SlackConfig
	for _, rcv := range cfg.Receivers {
		if rcv.Name == r.FormValue("receiver") {
			if len(rcv.SlackConfigs) == 0 {
				http.Error(w, fmt.Sprintf("no Slack notification configuration found for receiver %q", rcv.Name), http.StatusBadRequest)
				return
			}
			slackCfg = rcv.SlackConfigs[0]
			ctx = notify.WithReceiverName(ctx, rcv.Name)
			break
		}
	}
	if slackCfg == nil {
		http.Error(w, fmt.Sprintf("receiver %q not found", r.FormValue("receiver")), http.StatusBadRequest)
		return
	}

	if len(cfg.Templates) != 0 {
		http.Error(w, "external template files are not supported", http.StatusBadRequest)
		return
	}
	tmpl, err := template.FromGlobs(cfg.Templates...)
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading templates: %v", err), http.StatusBadRequest)
		return
	}
	u, err := url.Parse("https://alertmanager.local/")
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing external URL: %v", err), http.StatusBadRequest)
		return
	}
	tmpl.ExternalURL = u

	var msgBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("error reading request body: %v", err), http.StatusInternalServerError)
			return
		}

		msgBody = body
	}))
	defer srv.Close()

	u, err = url.Parse(srv.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing test server URL: %v", err), http.StatusInternalServerError)
		return
	}

	slackCfg.APIURL = (*config.SecretURL)(&config.URL{URL: u})
	n, err := notify.NewSlack(slackCfg, tmpl, h.logger)
	if err != nil {
		http.Error(w, fmt.Sprintf("error creating Slack notifier: %v", err), http.StatusInternalServerError)
		return
	}
	_, err = n.Notify(ctx, as...)
	if err != nil {
		http.Error(w, fmt.Sprintf("error sending notification: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(msgBody); err != nil {
		http.Error(w, fmt.Sprintf("error sending response: %v", err), http.StatusInternalServerError)
		return
	}
}

func main() {
	listenAddress := kingpin.Flag("web.listen-address", "Address to listen on for the web interface and API.").Default(":12345").String()
	var promlogConfig promlog.Config
	promlogflag.AddFlags(kingpin.CommandLine, &promlogConfig)
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logger := promlog.New(&promlogConfig)

	http.Handle("/preview", previewHandler{logger: logger})
	http.ListenAndServe(*listenAddress, nil)
}
