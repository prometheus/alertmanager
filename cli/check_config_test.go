package cli

import (
	"testing"
)

func TestCheckConfig(t *testing.T) {
	err := CheckConfig([]string{"testdata/conf.good.yml"})
	if err != nil {
		t.Fatalf("Checking valid config file failed with: %v", err)
	}

	err = CheckConfig([]string{"testdata/conf.bad.yml"})
	if err == nil {
		t.Fatalf("Failed to detect invalid file.")
	}
}
