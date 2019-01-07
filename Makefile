all: yaim

yaim: *.go
	go build -ldflags="-s -w" .

clean:
	rm -rf yaim