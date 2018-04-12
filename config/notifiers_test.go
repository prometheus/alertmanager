package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestEmailToIsPresent(t *testing.T) {
	in := `
to: ''
`
	var cfg EmailConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing to address in email config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestEmailHeadersCollision(t *testing.T) {
	in := `
to: 'to@email.com'
headers:
  Subject: 'Alert'
  subject: 'New Alert'
`
	var cfg EmailConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "duplicate header \"Subject\" in email config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPagerdutyRoutingKeyIsPresent(t *testing.T) {
	in := `
routing_key: ''
`
	var cfg PagerdutyConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing service or routing key in PagerDuty config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPagerdutyServiceKeyIsPresent(t *testing.T) {
	in := `
service_key: ''
`
	var cfg PagerdutyConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing service or routing key in PagerDuty config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestHipchatRoomIDIsPresent(t *testing.T) {
	in := `
room_id: ''
`
	var cfg HipchatConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing room id in Hipchat config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookURLIsPresent(t *testing.T) {
	in := `
url: ''
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing URL in webhook config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookHttpConfigIsValid(t *testing.T) {
	in := `
url: 'http://example.com'
http_config:
  bearer_token: foo
  bearer_token_file: /tmp/bar
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "at most one of bearer_token & bearer_token_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookHttpConfigIsOptional(t *testing.T) {
	in := `
url: 'http://example.com'
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	if err != nil {
		t.Fatalf("no error expected, returned:\n%v", err.Error())
	}
}

func TestWebhookPasswordIsObsfucated(t *testing.T) {
	in := `
url: 'http://example.com'
http_config:
  basic_auth:
    username: foo
    password: supersecret
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	if err != nil {
		t.Fatalf("no error expected, returned:\n%v", err.Error())
	}

	ycfg, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("no error expected, returned:\n%v", err.Error())
	}
	if strings.Contains(string(ycfg), "supersecret") {
		t.Errorf("Found password in the YAML cfg: %s\n", ycfg)
	}
}

func TestWechatAPIKeyIsPresent(t *testing.T) {
	in := `
api_secret: ''
`
	var cfg WechatConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing Wechat APISecret in Wechat config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}
func TestWechatCorpIDIsPresent(t *testing.T) {
	in := `
api_secret: 'api_secret'
corp_id: ''
`
	var cfg WechatConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing Wechat CorpID in Wechat config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestVictorOpsRoutingKeyIsPresent(t *testing.T) {
	in := `
routing_key: ''
`
	var cfg VictorOpsConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing Routing key in VictorOps config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPushoverUserKeyIsPresent(t *testing.T) {
	in := `
user_key: ''
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing user key in Pushover config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPushoverTokenIsPresent(t *testing.T) {
	in := `
user_key: '<user_key>'
token: ''
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing token in Pushover config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}
