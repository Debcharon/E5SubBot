package microsoft

import (
	"fmt"
	"net/url"
)

const (
	apiURL   string = "https://login.microsoftonline.com"
	graphURL string = "https://graph.microsoft.com"
	redirect string = "http://localhost/e5sub"
	scope    string = "openid offline_access mail.read user.read"
)

func GetAuthURL(clientID string) string {
	return fmt.Sprintf(
		"https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=%s&response_type=code&redirect_uri=%s&response_mode=query&scope=%s",
		clientID,
		url.QueryEscape(redirect),
		url.QueryEscape(scope),
	)
}

func GetRegURL() string {
	appUrl := fmt.Sprintf("https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade")
	return appUrl
}
