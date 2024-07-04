package gandi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/pya789/lego/v4/platform/tester"
	"github.com/stretchr/testify/require"
)

var envTest = tester.NewEnvTest(EnvAPIKey)

func TestNewDNSProvider(t *testing.T) {
	testCases := []struct {
		desc     string
		envVars  map[string]string
		expected string
	}{
		{
			desc: "success",
			envVars: map[string]string{
				EnvAPIKey: "123",
			},
		},
		{
			desc: "missing api key",
			envVars: map[string]string{
				EnvAPIKey: "",
			},
			expected: "gandi: some credentials information are missing: GANDI_API_KEY",
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			defer envTest.RestoreEnv()
			envTest.ClearEnv()

			envTest.Apply(test.envVars)

			p, err := NewDNSProvider()

			if test.expected == "" {
				require.NoError(t, err)
				require.NotNil(t, p)
				require.NotNil(t, p.config)
				require.NotNil(t, p.inProgressFQDNs)
				require.NotNil(t, p.inProgressAuthZones)
			} else {
				require.EqualError(t, err, test.expected)
			}
		})
	}
}

func TestNewDNSProviderConfig(t *testing.T) {
	testCases := []struct {
		desc     string
		apiKey   string
		expected string
	}{
		{
			desc:   "success",
			apiKey: "123",
		},
		{
			desc:     "missing credentials",
			expected: "gandi: no API Key given",
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			config := NewDefaultConfig()
			config.APIKey = test.apiKey

			p, err := NewDNSProviderConfig(config)

			if test.expected == "" {
				require.NoError(t, err)
				require.NotNil(t, p)
				require.NotNil(t, p.config)
				require.NotNil(t, p.inProgressFQDNs)
				require.NotNil(t, p.inProgressAuthZones)
			} else {
				require.EqualError(t, err, test.expected)
			}
		})
	}
}

// TestDNSProvider runs Present and CleanUp against a fake Gandi RPC
// Server, whose responses are predetermined for particular requests.
func TestDNSProvider(t *testing.T) {
	// serverResponses is the XML-RPC Request->Response map used by the
	// fake RPC server. It was generated by recording a real RPC session
	// which resulted in the successful issue of a cert, and then
	// anonymizing the RPC data.
	serverResponses := map[string]string{
		// Present Request->Response 1 (getZoneID)
		presentGetZoneIDRequestMock: presentGetZoneIDResponseMock,
		// Present Request->Response 2 (cloneZone)
		presentCloneZoneRequestMock: presentCloneZoneResponseMock,
		// Present Request->Response 3 (newZoneVersion)
		presentNewZoneVersionRequestMock: presentNewZoneVersionResponseMock,
		// Present Request->Response 4 (addTXTRecord)
		presentAddTXTRecordRequestMock: presentAddTXTRecordResponseMock,
		// Present Request->Response 5 (setZoneVersion)
		presentSetZoneVersionRequestMock: presentSetZoneVersionResponseMock,
		// Present Request->Response 6 (setZone)
		presentSetZoneRequestMock: presentSetZoneResponseMock,
		// CleanUp Request->Response 1 (setZone)
		cleanupSetZoneRequestMock: cleanupSetZoneResponseMock,
		// CleanUp Request->Response 2 (deleteZone)
		cleanupDeleteZoneRequestMock: cleanupDeleteZoneResponseMock,
	}

	fakeKeyAuth := "XXXX"

	regexpDate := regexp.MustCompile(`\[ACME Challenge [^\]:]*:[^\]]*\]`)

	// start fake RPC server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "text/xml", r.Header.Get("Content-Type"), "invalid content type")

		req, errS := io.ReadAll(r.Body)
		require.NoError(t, errS)

		req = regexpDate.ReplaceAllLiteral(req, []byte(`[ACME Challenge 01 Jan 16 00:00 +0000]`))
		resp, ok := serverResponses[string(req)]
		require.Truef(t, ok, "Server response for request not found: %s", string(req))

		_, errS = io.Copy(w, strings.NewReader(resp))
		require.NoError(t, errS)
	}))
	t.Cleanup(server.Close)

	// define function to override findZoneByFqdn with
	fakeFindZoneByFqdn := func(fqdn string) (string, error) {
		return "example.com.", nil
	}

	config := NewDefaultConfig()
	config.BaseURL = server.URL + "/"
	config.APIKey = "123412341234123412341234"

	provider, err := NewDNSProviderConfig(config)
	require.NoError(t, err)

	// override findZoneByFqdn function
	savedFindZoneByFqdn := provider.findZoneByFqdn
	t.Cleanup(func() {
		provider.findZoneByFqdn = savedFindZoneByFqdn
	})
	provider.findZoneByFqdn = fakeFindZoneByFqdn

	// run Present
	err = provider.Present("abc.def.example.com", "", fakeKeyAuth)
	require.NoError(t, err)

	// run CleanUp
	err = provider.CleanUp("abc.def.example.com", "", fakeKeyAuth)
	require.NoError(t, err)
}
