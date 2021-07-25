# lido
[![Build Status](https://github.com/stevecallear/lido/actions/workflows/build.yml/badge.svg)](https://github.com/stevecallear/lido/actions/workflows/build.yml)
[![codecov](https://codecov.io/gh/stevecallear/lido/branch/master/graph/badge.svg)](https://codecov.io/gh/stevecallear/lido)
[![Go Report Card](https://goreportcard.com/badge/github.com/stevecallear/lido)](https://goreportcard.com/report/github.com/stevecallear/lido)

Lido is a simple, thread-safe pool that uses buffered channels.

## Getting started
```
go get github.com/stevecallear/lido@latest
```
```
pool := lido.New(lido.Options{
	New: func() (interface{}, error) {
		return "value", nil
	},
})

item, err := pool.Next()
if err != nil {
	panic(err)
}
defer item.Restore()

fmt.Println(item.Value())
// Output: value
```

## Pool options
Pool size and timeout can be specified in the supplied options. Calls to `Next` will return `ErrTimeout` if a pool item is not available before the timeout has elapsed.
```
pool := lido.New(lido.Options{
	New: func() (interface{}, error) {
		return "value", nil
	},
	Size:    10,              // defaults to 1
	Timeout: 1 * time.Second, // defaults to 30 seconds
})
```

## Replacing pool items
If a pool item is unusable it can be removed from the pool by invoking `Remove` on the `Item`. This will create a free pool slot that will be populated when required.

An item can only be removed or restored once. Further calls will result in a panic.

```
item, err := pool.Next()
if err != nil {
	return err
}

err = item.Value().(DB).Ping()
if err != nil {
	item.Remove()
	return err
}

defer item.Restore()
// ...
```