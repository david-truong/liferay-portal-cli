package main

import "github.com/david-truong/liferay-portal-cli/internal/cli"

var version = "dev"

func main() {
	cli.Execute(version)
}
