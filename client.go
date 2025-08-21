package httputil

import (
	"net/http"
	"strings"
)

type Client struct {
	basePath string
	client   *http.Client
}

func NewClient(options ...ClientOption) Client {
	opts := mapClientOptionsToDefaults(options)

	return Client{
		basePath: strings.TrimRight(opts.basePath, "/"),
		client: &http.Client{
			CheckRedirect: opts.checkRedirect,
			Jar:           opts.jar,
			Timeout:       opts.timeout,
		},
	}
}
