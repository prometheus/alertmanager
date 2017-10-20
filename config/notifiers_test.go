package config

import (
	"testing"
	"gopkg.in/yaml.v2"
)

func TestEmailToIsPresent(t *testing.T) {
	in := `
to: ''
`
	var cfg EmailConfig
	err := yaml.Unmarshal([]byte(in), &cfg)

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
	err := yaml.Unmarshal([]byte(in), &cfg)

	expected := "duplicate header \"Subject\" in email config"

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
	err := yaml.Unmarshal([]byte(in), &cfg)

	expected := "missing service key in PagerDuty config"

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
	err := yaml.Unmarshal([]byte(in), &cfg)

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
	err := yaml.Unmarshal([]byte(in), &cfg)

	expected := "missing URL in webhook config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestOpsGenieAPIKeyIsPresent(t *testing.T) {
	in := `
api_key: ''
`
	var cfg OpsGenieConfig
	err := yaml.Unmarshal([]byte(in), &cfg)

	expected := "missing API key in OpsGenie config"

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
	err := yaml.Unmarshal([]byte(in), &cfg)

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
	err := yaml.Unmarshal([]byte(in), &cfg)

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
	err := yaml.Unmarshal([]byte(in), &cfg)

	expected := "missing token in Pushover config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}