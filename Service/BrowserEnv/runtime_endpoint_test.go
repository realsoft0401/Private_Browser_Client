package BrowserEnv

import (
	"testing"

	"private_browser_client/Settings"
)

func TestPublishedPortAddressForServiceUsesDockerAPIHost(t *testing.T) {
	oldConf := Settings.Conf
	defer func() { Settings.Conf = oldConf }()

	Settings.Conf = &Settings.AppConfig{
		DockerConfig: &Settings.DockerConfig{APIURL: "http://host.docker.internal:2375"},
	}

	if got := publishedPortAddressForService(9104); got != "host.docker.internal:9104" {
		t.Fatalf("expected host.docker.internal:9104, got %s", got)
	}
}

func TestPublishedPortAddressForServiceFallsBackToLoopback(t *testing.T) {
	oldConf := Settings.Conf
	defer func() { Settings.Conf = oldConf }()

	Settings.Conf = &Settings.AppConfig{
		DockerConfig: &Settings.DockerConfig{APIURL: "http://localhost:2375"},
	}

	if got := publishedPortAddressForService(9104); got != "127.0.0.1:9104" {
		t.Fatalf("expected 127.0.0.1:9104, got %s", got)
	}
}

func TestRewriteCDPWebSocketURLForService(t *testing.T) {
	oldConf := Settings.Conf
	defer func() { Settings.Conf = oldConf }()

	Settings.Conf = &Settings.AppConfig{
		DockerConfig: &Settings.DockerConfig{APIURL: "http://192.168.10.119:2375"},
	}

	got := rewriteCDPWebSocketURLForService("ws://127.0.0.1:8104/devtools/page/abc?x=1", 8104)
	want := "ws://192.168.10.119:8104/devtools/page/abc?x=1"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
