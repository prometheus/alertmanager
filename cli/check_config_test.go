package cli

import (
	"testing"
)

func TestCheckConfig(t *testing.T) {
	err := CheckConfig([]string{"testdata/conf.good.yml"})
	if err != nil {
		t.Fatalf("checking valid config file failed with: %v", err)
	}

	err = CheckConfig([]string{"testdata/conf.bad.yml"})
	if err == nil {
		t.Fatalf("failed to detect invalid file.")
	}
}
