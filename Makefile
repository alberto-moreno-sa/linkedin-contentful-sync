.PHONY: build run-scrape run-list clean

build:
	go build -o bin/linkedin-sync .

run-scrape:
	go run . scrape --profile=$(PROFILE)

run-list:
	go run . list

clean:
	rm -rf bin/
