package main

import "github.com/zalando/go-keyring"

const keychainService = "ccc"

func keychainGet(provider string) (string, error) {
	return keyring.Get(keychainService, provider)
}

func keychainSet(provider, token string) error {
	return keyring.Set(keychainService, provider, token)
}

func keychainDelete(provider string) error {
	return keyring.Delete(keychainService, provider)
}
