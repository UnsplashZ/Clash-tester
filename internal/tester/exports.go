package tester

import (
	"Clash-tester/pkg/models"
	"net/http"
	"net/url"
	"time"
)

// Export helper functions for server package
func CreateProxyClient(proxyURL string) *http.Client {
	proxyURLParsed, _ := url.Parse(proxyURL)

	return &http.Client{
		Timeout: TestTimeout,
		Transport: &http.Transport{
			Proxy:              http.ProxyURL(proxyURLParsed),
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}
}

func TestServiceWithRetry(client *http.Client, serviceName string, fn testFunc) models.ServiceTest {
	return testServiceWithRetry(client, serviceName, fn)
}

func TestOpenAI(client *http.Client, result *models.ServiceTest) error {
	return testOpenAI(client, result)
}

func TestGemini(client *http.Client, result *models.ServiceTest) error {
	return testGemini(client, result)
}

func TestClaude(client *http.Client, result *models.ServiceTest) error {
	return testClaude(client, result)
}
