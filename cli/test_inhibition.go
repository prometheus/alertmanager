package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

const inhibitionHelp = `Commands related to inhibition handling`

const inhibitionTestHelp = `Test alert inhibition

This command will assert that the tested alerts are inhibited given
the labels of the firing alerts.

Inhibitions are loaded from a local configuration file or a running Alertmanager configuration.
Specifying --config.file takes precedence over --alertmanager.url.

Example:

./amtool config inhibition test --config.file=doc/examples/simple.yml --verify.inhibitions.file=doc/examples/test-inhibition.yml
`

type inhibition struct {
	configFile string

	assertFile string
}

type verifyConfig struct {
	Given    []config.Matchers `yaml:"given"`
	Inhibits []config.Matchers `yaml:"inhibits"`
	Alerts   []config.Matchers `yaml:"alerts"`
}

type inhibitConfig struct {
	Verify []verifyConfig `yaml:"verify"`
}

func configureInhibitionCmd(app *kingpin.CmdClause) {
	var (
		c             = &inhibition{}
		inhibitionCmd = app.Command("inhibition", inhibitionHelp)
		configFlag    = inhibitionCmd.Flag("config.file", "Config file to be tested.")
	)
	configFlag.ExistingFileVar(&c.configFile)
	configureInhibutionTestCmd(inhibitionCmd, c)
}

func configureInhibutionTestCmd(cc *kingpin.CmdClause, i *inhibition) {
	testCmd := cc.Command("test", inhibitionTestHelp)
	verifyFlag := testCmd.Flag("verify.inhibitions.file", "File to test assertions.")
	verifyFlag.ExistingFileVar(&i.assertFile)

	testCmd.Action(execWithTimeout(i.inhibitionTestAction))
}

func toLabelSet(ms config.Matchers) (model.LabelSet, error) {
	lbl := model.LabelSet{}
	for _, m := range ms {
		if m.Type != labels.MatchEqual {
			return model.LabelSet{}, fmt.Errorf("match must be equal. was %v", m)
		}
		lbl[model.LabelName(m.Name)] = model.LabelValue(m.Value)
	}
	return lbl, lbl.Validate()
}

func toAlert(ms config.Matchers) (*types.Alert, error) {
	lbls, err := toLabelSet(ms)
	if err != nil {
		return nil, err
	}
	return &types.Alert{
		Alert: model.Alert{
			Labels: lbls,
		},
		UpdatedAt: time.Now(),
	}, nil
}

func verifyInhibition(ctx context.Context, cfg *config.Config, v *verifyConfig) error {
	l := promslog.NewNopLogger()
	marker := types.NewMarker(prometheus.DefaultRegisterer)
	s, err := mem.NewAlerts(ctx, marker, time.Minute, nil, l, prometheus.DefaultRegisterer)
	if err != nil {
		return fmt.Errorf("failed to create alert backend: %w", err)
	}
	defer s.Close()

	for _, m := range v.Given {
		a, err := toAlert(m)
		if err != nil {
			return fmt.Errorf("failed to create alert: %w", err)
		}
		if err := s.Put(ctx, a); err != nil {
			return fmt.Errorf("failed to store alert %v: %w", a, err)
		}
	}

	inh := inhibit.NewInhibitor(s, cfg.InhibitRules, marker, l)
	go inh.Run()
	inh.WaitForLoading()
	defer inh.Stop()

	errs := make([]error, 0)
	matches := 0

	for _, inhibited := range v.Inhibits {
		lbl, err := toLabelSet(inhibited)
		if err != nil {
			return fmt.Errorf("failed to create labels %v: %w", inhibited, err)
		}
		if !inh.Mutes(ctx, lbl) {
			errs = append(errs, fmt.Errorf("Labels %v are not inhibited", lbl))
		}
		matches++
	}

	for _, alerting := range v.Alerts {
		lbl, err := toLabelSet(alerting)
		if err != nil {
			return fmt.Errorf("failed to create labels %v: %w", alerting, err)
		}
		if inh.Mutes(ctx, lbl) {
			errs = append(errs, fmt.Errorf("Labels %v are wrongly inhibited", lbl))
		}
		matches++
	}

	if matches == 0 {
		return fmt.Errorf("Neither positive nor negative assertion present. Have at least one")
	}

	return errors.Join(errs...)
}

func (i *inhibition) inhibitionTestAction(ctx context.Context, _ *kingpin.ParseContext) error {
	iCfg := &inhibitConfig{}
	b, err := os.ReadFile(i.assertFile)
	if err != nil {
		kingpin.Fatalf("Failed to open %q: %v\n", i.assertFile, err)
		return err
	}
	if err := yaml.UnmarshalStrict(b, iCfg); err != nil {
		kingpin.Fatalf("Failed to parse %q: %v\n", i.assertFile, err)
		return err
	}

	cfg, err := loadAlertmanagerConfig(ctx, alertmanagerURL, i.configFile)
	if err != nil {
		kingpin.Fatalf("%v\n", err)
		return err
	}

	for _, v := range iCfg.Verify {
		if err := verifyInhibition(ctx, cfg, &v); err != nil {
			kingpin.Fatalf("%v\n", err)
			return err
		}
	}
	return nil
}
