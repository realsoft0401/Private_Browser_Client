package BrowserEnv

import (
	"testing"

	model "private_browser_client/Models/BrowserEnv"
)

func TestTimezoneProbeTransportUsesCDPForRuleMode(t *testing.T) {
	pkg := &runPackage{
		Profile: model.ProfileFile{
			Proxy: model.ProfileProxy{Enabled: true},
		},
		ProxyConfig: "mode: rule\nmixed-port: 7897\n",
	}

	if got := selectTimezoneProbeTransport(pkg); got != timezoneProbeTransportCDP {
		t.Fatalf("expected rule mode to use CDP, got %s", got)
	}
}

func TestTimezoneProbeTransportUsesCurlForGlobalAndDirect(t *testing.T) {
	cases := []string{"global", "direct"}
	for _, mode := range cases {
		t.Run(mode, func(t *testing.T) {
			pkg := &runPackage{
				Profile: model.ProfileFile{
					Proxy: model.ProfileProxy{Enabled: true},
				},
				ProxyConfig: "mode: " + mode + "\nmixed-port: 7897\n",
			}

			if got := selectTimezoneProbeTransport(pkg); got != timezoneProbeTransportCurl {
				t.Fatalf("expected %s mode to use curl, got %s", mode, got)
			}
		})
	}
}

func TestTimezoneProbeTransportUsesCurlWhenProxyDisabled(t *testing.T) {
	pkg := &runPackage{
		Profile: model.ProfileFile{
			Proxy: model.ProfileProxy{Enabled: false},
		},
		ProxyConfig: "mode: rule\nmixed-port: 7897\n",
	}

	if got := selectTimezoneProbeTransport(pkg); got != timezoneProbeTransportCurl {
		t.Fatalf("expected disabled proxy to use curl, got %s", got)
	}
}
