package main

import "github.com/david-truong/liferay-portal-cli/cmd"

var version = "dev"

func main() {
	cmd.Execute(version)
}
