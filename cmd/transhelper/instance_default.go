//go:build windows && !uidrive

package main

func instanceName() (string, error) {
	return `Local\transHelper`, nil
}
