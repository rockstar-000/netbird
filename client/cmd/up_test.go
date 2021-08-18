package cmd

import (
	"errors"
	"fmt"
	"github.com/wiretrustee/wiretrustee/iface"
	mgmt "github.com/wiretrustee/wiretrustee/management/server"
	"github.com/wiretrustee/wiretrustee/util"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var signalAddr string

func TestUp_Start(t *testing.T) {
	config := &mgmt.Config{}
	_, err := util.ReadJson("../testdata/management.json", config)
	if err != nil {
		t.Fatal(err)
	}
	testDir := t.TempDir()
	config.Datadir = testDir
	err = util.CopyFileContents("../testdata/store.json", filepath.Join(testDir, "store.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, signalLis := startSignal(t)
	signalAddr = signalLis.Addr().String()
	config.Signal.URI = signalAddr

	_, mgmLis := startManagement(config, t)
	mgmAddr = mgmLis.Addr().String()

}

func TestUp_ShouldFail_On_NoConfig(t *testing.T) {

	tempDir := t.TempDir()
	confPath := tempDir + "/config.json"
	mgmtURL := fmt.Sprintf("http://%s", mgmAddr)
	rootCmd.SetArgs([]string{
		"up",
		"--config",
		confPath,
		"--management-url",
		mgmtURL,
	})
	err := rootCmd.Execute()
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expecting login command to fail on absence of config")
	}
}

func TestUp(t *testing.T) {

	defer iface.Close()

	tempDir := t.TempDir()
	confPath := tempDir + "/config.json"
	mgmtURL, err := url.Parse("http://" + mgmAddr)
	if err != nil {
		t.Fatal(err)
	}
	rootCmd.SetArgs([]string{
		"login",
		"--config",
		confPath,
		"--setup-key",
		"a2c8e62b-38f5-4553-b31e-dd66c696cebb",
		"--management-url",
		mgmtURL.String(),
	})
	err = rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	rootCmd.SetArgs([]string{
		"up",
		"--config",
		confPath,
	})
	go func() {
		err = rootCmd.Execute()
		if err != nil {
			t.Errorf("expected no error while running up command, got %v", err)
		}
	}()

	exists := false
	for start := time.Now(); time.Since(start) < 7*time.Second; {
		e, err := iface.Exists(iface.WgInterfaceDefault)
		if err != nil {
			continue
		}
		if *e {
			exists = true
			break
		}

	}

	if !exists {
		t.Errorf("expected wireguard interface %s to be created", iface.WgInterfaceDefault)
	}
}
