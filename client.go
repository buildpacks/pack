package pack

import (
	"github.com/buildpack/pack/config"
)

type Client struct {
	config  *config.Config
	fetcher ImageFetcher
}

func NewClient(config *config.Config, fetcher ImageFetcher) *Client {
	return &Client{
		config:  config,
		fetcher: fetcher,
	}
}

//// TODO : move to build.go
//func (c *Client) Build() {
//
//}
//
//// TODO : move to create_builder.go
//func (c *Client) CreateBuilder() {
//
//}
//

//// TODO : move to run.go
//func (c *Client) Run() {
//
//}

// TODO : move to rebase.go
//func (c *Client) Rebase() {
//
//}
