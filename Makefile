test:
	go test -v

cover:
	go test -coverprofile=coverage.out
	go tool cover -func=coverage.out
	rm coverage.out