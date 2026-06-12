//go:build !windows

package main

type singleInstanceLock struct{}

func acquireSingleInstance() (*singleInstanceLock, bool, error) {
	return &singleInstanceLock{}, false, nil
}

func (l *singleInstanceLock) Release() {}

func showAlreadyRunningMessage(title, message string) {}
